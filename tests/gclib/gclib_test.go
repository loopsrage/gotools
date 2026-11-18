package gclib_test

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/gclib"
	"github.com/loopsrage/gotools/lib/sqllib"
	"github.com/loopsrage/gotools/tests/internal/sqlitedriver"
)

func generateServiceAccountJSON(t *testing.T, tokenURI string) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	sa := map[string]string{
		"type":         "service_account",
		"client_email": "svc@test-proj.iam.gserviceaccount.com",
		"private_key":  string(pemBytes),
		"token_uri":    tokenURI,
	}
	b, _ := json.Marshal(sa)
	return b
}

type vertexTestServer struct {
	aiplatformpb.UnimplementedPredictionServiceServer
	aiplatformpb.UnimplementedEndpointServiceServer
}

func (v *vertexTestServer) Predict(_ context.Context, req *aiplatformpb.PredictRequest) (*aiplatformpb.PredictResponse, error) {
	if req.GetEndpoint() != "projects/my-project/locations/us-central1/endpoints/123" {
		return nil, status.Errorf(codes.NotFound, "endpoint not found: %s", req.GetEndpoint())
	}
	if len(req.GetInstances()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing instances")
	}
	predVal, err := structpb.NewValue(map[string]interface{}{"score": 0.9})
	if err != nil {
		return nil, err
	}
	return &aiplatformpb.PredictResponse{
		Predictions: []*structpb.Value{predVal},
	}, nil
}

func (v *vertexTestServer) ListEndpoints(_ context.Context, req *aiplatformpb.ListEndpointsRequest) (*aiplatformpb.ListEndpointsResponse, error) {
	if req.GetParent() != "projects/my-project/locations/us-central1" {
		return nil, status.Errorf(codes.InvalidArgument, "unexpected parent: %s", req.GetParent())
	}
	return &aiplatformpb.ListEndpointsResponse{
		Endpoints: []*aiplatformpb.Endpoint{
			{Name: "ep1", DisplayName: "First"},
		},
	}, nil
}

func TestGcpAiSDKAndServiceAccountTokenSource(t *testing.T) {
	ctx := context.Background()

	// Verify TokenSourceFromJSON performs real RS256 JWT exchange via golang.org/x/oauth2/google
	tokSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if !strings.Contains(r.Form.Get("grant_type"), "jwt-bearer") {
			http.Error(w, "invalid grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "sa-jwt-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokSrv.Close()

	saJSON := generateServiceAccountJSON(t, tokSrv.URL)
	ts, err := gclib.TokenSourceFromJSON(ctx, saJSON, tokSrv.Client())
	if err != nil {
		t.Fatalf("TokenSourceFromJSON error: %v", err)
	}
	tok, err := ts.Token()
	if err != nil || tok.AccessToken != "sa-jwt-access-token" {
		t.Fatalf("TokenSource.Token() = (%+v, %v)", tok, err)
	}

	// Spin up real gRPC server for cloud.google.com/go/aiplatform/apiv1
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen error: %v", err)
	}
	grpcSrv := grpc.NewServer()
	vImpl := &vertexTestServer{}
	aiplatformpb.RegisterPredictionServiceServer(grpcSrv, vImpl)
	aiplatformpb.RegisterEndpointServiceServer(grpcSrv, vImpl)
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient error: %v", err)
	}
	defer conn.Close()

	ai, err := gclib.NewGcpAi(gclib.GcpAiConfig{
		ProjectID: "my-project",
		Location:  "us-central1",
		GRPCConn:  conn,
	})
	if err != nil {
		t.Fatalf("NewGcpAi error: %v", err)
	}
	defer ai.Close()

	preds, err := ai.Predict(ctx, "projects/my-project/locations/us-central1/endpoints/123", []map[string]interface{}{{"a": 1}})
	if err != nil || len(preds) != 1 {
		t.Fatalf("Predict full path = (%v, %v)", preds, err)
	}
	predMap := preds[0].(map[string]interface{})
	if predMap["score"].(float64) != 0.9 {
		t.Fatalf("Predict score = %v, want 0.9", predMap["score"])
	}

	predsShort, err := ai.Predict(ctx, "123", []map[string]interface{}{{"a": 1}})
	if err != nil || len(predsShort) != 1 {
		t.Fatalf("Predict short ID = (%v, %v)", predsShort, err)
	}

	if _, err := ai.Predict(ctx, "missing-ep", []map[string]interface{}{{"a": 1}}); err == nil {
		t.Fatal("expected gRPC error on missing endpoint")
	}

	eps, err := ai.ListEndpoints(ctx)
	if err != nil || len(eps) != 1 || eps[0].Name != "ep1" || eps[0].DisplayName != "First" {
		t.Fatalf("ListEndpoints = (%+v, %v)", eps, err)
	}
}

func extractGCSUpload(t *testing.T, r *http.Request) (string, []byte) {
	t.Helper()
	name := r.URL.Query().Get("name")
	ct := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err == nil && strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(r.Body, params["boundary"])
		var meta struct {
			Name string `json:"name"`
		}
		part1, err := mr.NextPart()
		if err != nil {
			t.Fatalf("multipart part1 error: %v", err)
		}
		_ = json.NewDecoder(part1).Decode(&meta)
		if meta.Name != "" {
			name = meta.Name
		}
		part2, err := mr.NextPart()
		if err != nil {
			t.Fatalf("multipart part2 error: %v", err)
		}
		payload, _ := io.ReadAll(part2)
		return name, payload
	}
	payload, _ := io.ReadAll(r.Body)
	return name, payload
}

func TestGcsBlobSDKAndFilesystemIndex(t *testing.T) {
	ctx := context.Background()
	objects := make(map[string][]byte)
	var uploadCount int

	mux := http.NewServeMux()
	mux.HandleFunc("/upload/storage/v1/b/my-bucket/o", func(w http.ResponseWriter, r *http.Request) {
		uploadCount++
		name, data := extractGCSUpload(t, r)
		objects[name] = data
		sum := md5.Sum(data)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"bucket":  "my-bucket",
			"name":    name,
			"size":    strconv.Itoa(len(data)),
			"md5Hash": base64.StdEncoding.EncodeToString(sum[:]),
			"etag":    `"` + fileslib.CalculateLocalMD5(data) + `"`,
		})
	})
	mux.HandleFunc("/storage/v1/b/my-bucket/o", func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("prefix")
		var items []map[string]interface{}
		for k, data := range objects {
			if strings.HasPrefix(k, prefix) {
				sum := md5.Sum(data)
				items = append(items, map[string]interface{}{
					"bucket":  "my-bucket",
					"name":    k,
					"size":    strconv.Itoa(len(data)),
					"md5Hash": base64.StdEncoding.EncodeToString(sum[:]),
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": items})
	})
	mux.HandleFunc("/storage/v1/b/my-bucket/o/", func(w http.ResponseWriter, r *http.Request) {
		obj := strings.TrimPrefix(r.URL.Path, "/storage/v1/b/my-bucket/o/")
		data, ok := objects[obj]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("alt") == "media" {
			_, _ = w.Write(data)
			return
		}
		sum := md5.Sum(data)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"bucket":  "my-bucket",
			"name":    obj,
			"size":    strconv.Itoa(len(data)),
			"md5Hash": base64.StdEncoding.EncodeToString(sum[:]),
			"etag":    `"` + fileslib.CalculateLocalMD5(data) + `"`,
		})
	})
	mux.HandleFunc("/my-bucket/", func(w http.ResponseWriter, r *http.Request) {
		obj := strings.TrimPrefix(r.URL.Path, "/my-bucket/")
		data, ok := objects[obj]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	blob, err := gclib.NewGcsBlob(gclib.GcsBlobConfig{
		BucketName:  "my-bucket",
		Project:     "my-project",
		Token:       "gcs-tok",
		EndpointURL: srv.URL,
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewGcsBlob error: %v", err)
	}
	defer blob.Close()

	if blob.GcsClient() == nil {
		t.Fatal("GcsClient() should return non-nil *storage.Client")
	}
	if got := blob.FullKey("dir/x.txt"); got != "my-bucket/dir/x.txt" {
		t.Fatalf("FullKey = %q", got)
	}

	wrote, err := blob.WriteIfDifferent(ctx, "dir/x.txt", []byte("data"))
	if err != nil || !wrote || uploadCount != 1 {
		t.Fatalf("first WriteIfDifferent = (%v, %v), uploads=%d", wrote, err, uploadCount)
	}
	wrote2, err := blob.WriteIfDifferent(ctx, "dir/x.txt", []byte("data"))
	if err != nil || wrote2 || uploadCount != 1 {
		t.Fatalf("second WriteIfDifferent = (%v, %v), uploads=%d", wrote2, err, uploadCount)
	}

	d, loaded, err := blob.LoadOrStore(ctx, "dir/x.txt", func() ([]byte, error) {
		return []byte("data"), nil
	})
	if err != nil || !loaded || string(d) != "data" {
		t.Fatalf("LoadOrStore = (%q, %v, %v)", string(d), loaded, err)
	}

	// Transfer from real GCS SDK client to LocalFS via FilesystemIndex
	localFS := fileslib.NewLocalFS()
	fss := fileslib.NewFilesystemIndex()
	fss.RegisterFilesystems(map[string]fileslib.FS{"local": localFS, "gcs": blob})

	gcsFS, _ := fss.Filesystem("gcs")
	lcFS, _ := fss.Filesystem("local")

	outDir := t.TempDir()
	if err := gcsFS.Transfer(ctx, lcFS, "dir/*", outDir); err != nil {
		t.Fatalf("gcsFS.Transfer error: %v", err)
	}
	gotBytes, err := os.ReadFile(filepath.Join(outDir, "x.txt"))
	if err != nil || string(gotBytes) != "data" {
		t.Fatalf("transferred local file = %q, err=%v", string(gotBytes), err)
	}
}

func TestGcpSecretsSDKRESTClient(t *testing.T) {
	ctx := context.Background()
	var versionAdded string

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/projects/my-project/secrets/my-secret/versions/latest:access", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"name": "projects/my-project/secrets/my-secret/versions/1",
			"payload": map[string]string{
				"data": base64.StdEncoding.EncodeToString([]byte("top-secret")),
			},
		})
	})
	mux.HandleFunc("/v1/projects/my-project/secrets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			id := r.URL.Query().Get("secretId")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"name": "projects/my-project/secrets/" + id,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"secrets": []map[string]interface{}{
				{"name": "projects/my-project/secrets/my-secret"},
			},
		})
	})
	mux.HandleFunc("/v1/projects/my-project/secrets/my-secret:addVersion", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Payload struct {
				Data string `json:"data"`
			} `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		decoded, _ := base64.StdEncoding.DecodeString(body.Payload.Data)
		versionAdded = string(decoded)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name": "projects/my-project/secrets/my-secret/versions/1",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sec, err := gclib.NewGcpSecrets(gclib.GcpSecretsConfig{
		ProjectID:   "my-project",
		Token:       "sec-tok",
		EndpointURL: srv.URL,
		HTTPClient:  srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewGcpSecrets error: %v", err)
	}
	defer sec.Close()

	val, err := sec.GetSecret(ctx, "my-secret")
	if err != nil || val != "top-secret" {
		t.Fatalf("GetSecret = (%q, %v), want top-secret", val, err)
	}
	if _, err := sec.GetSecret(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing secret")
	}

	putRes, err := sec.PutSecret(ctx, "my-secret", "new-val")
	if err != nil || putRes["name"] != "projects/my-project/secrets/my-secret" || versionAdded != "new-val" {
		t.Fatalf("PutSecret = (%v, %v), versionAdded=%q", putRes, err, versionAdded)
	}

	list, err := sec.ListSecrets(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListSecrets = (%v, %v)", list, err)
	}
}

func TestGcpSqlConfigInspectAndSearch(t *testing.T) {
	ctx := context.Background()
	cfg := gclib.GcpSqlConfig{
		Driver:                 "{PostgreSQL Unicode}",
		Server:                 "cloudsql.example.com",
		Database:               "prod_db",
		Username:               "postgres",
		Password:               "secret",
		InstanceConnectionName: "proj:region:instance",
	}
	cs := cfg.ConStr()
	if !strings.Contains(cs, "Server=tcp:cloudsql.example.com;") || !strings.Contains(cs, "Database=prod_db;") {
		t.Fatalf("unexpected ConStr: %q", cs)
	}

	// Verify default pgx driver registration opens cleanly
	pgPool, err := gclib.NewGcpSql("", "", cfg)
	if err != nil {
		t.Fatalf("NewGcpSql default pgx open failed: %v", err)
	}
	_ = pgPool.Close()

	pgOnly := gclib.NewGcpSqlWithDB(nil, cfg)
	qStr, qArgs, err := pgOnly.BuildSearchQuery(sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		Columns:    "*",
		OrderCol:   "Item",
		Filters:    map[string]interface{}{"Item": []int{2, 3}},
		Limit:      sqllib.IntPtr(10),
		LastItem:   1,
	})
	if err != nil {
		t.Fatalf("BuildSearchQuery(postgres) error: %v", err)
	}
	wantPG := "SELECT * FROM items AS base WHERE base.Item > $1 AND base.Item IN ($2, $3) ORDER BY Item LIMIT 10"
	if qStr != wantPG || len(qArgs) != 3 {
		t.Fatalf("BuildSearchQuery(postgres) = %q (%v), want %q", qStr, qArgs, wantPG)
	}

	memDB, err := sqlitedriver.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory error: %v", err)
	}
	defer memDB.Close()

	if _, err := memDB.ExecContext(ctx, "CREATE TABLE items (Item INTEGER PRIMARY KEY, Description TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE error: %v", err)
	}
	if _, err := memDB.ExecContext(ctx, "INSERT INTO items (Item, Description) VALUES (1, 'A'), (2, 'B'), (3, 'C')"); err != nil {
		t.Fatalf("INSERT error: %v", err)
	}

	db := gclib.NewGcpSqlWithDB(memDB, cfg)
	tables, err := db.InspectTables(ctx, "")
	if err != nil || len(tables) != 1 || tables[0] != "items" {
		t.Fatalf("InspectTables = (%v, %v)", tables, err)
	}

	meta, err := db.TableMetadata(ctx, "items", "")
	if err != nil || meta.Name != "items" || len(meta.Columns) != 2 {
		t.Fatalf("TableMetadata = (%+v, %v)", meta, err)
	}

	filtered, err := db.Search(ctx, sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		Columns:    "*",
		OrderCol:   "Item",
		Filters:    map[string]interface{}{"Item": []int{2, 3}},
		LastItem:   1,
	})
	if err != nil || len(filtered) != 2 || filtered[0]["Description"] != "B" || filtered[1]["Description"] != "C" {
		t.Fatalf("Search list filter = (%v, %v)", filtered, err)
	}
}
