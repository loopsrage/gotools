package awslib_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loopsrage/gotools/lib/awslib"
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/sqllib"
	"github.com/loopsrage/gotools/tests/internal/sqlitedriver"
)

func assertSigV4(t *testing.T, r *http.Request, expectedService string) {
	t.Helper()
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIA_TEST/") || !strings.Contains(auth, "/"+expectedService+"/aws4_request") {
		t.Errorf("invalid SigV4 Authorization header for %s: %q", expectedService, auth)
	}
	if r.Header.Get("X-Amz-Date") == "" {
		t.Errorf("missing X-Amz-Date header")
	}
	if expectedService == "s3" && r.Header.Get("X-Amz-Content-Sha256") == "" {
		t.Errorf("missing X-Amz-Content-Sha256 header on S3 request")
	}
}

func TestAwsAiBedrockSigV4InvokeAndListModels(t *testing.T) {
	ctx := context.Background()

	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	if _, err := awslib.NewAwsAi(awslib.AwsAiConfig{RegionName: "us-east-1"}); err == nil {
		t.Fatal("expected NewAwsAi to fail without IAM credentials")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/model/anthropic.claude-v2/invoke", func(w http.ResponseWriter, r *http.Request) {
		assertSigV4(t, r, "bedrock")
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["prompt"] != "prompt-text" || body["top_k"].(float64) != 40 {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"completion": "hello"})
	})
	mux.HandleFunc("/foundation-models", func(w http.ResponseWriter, r *http.Request) {
		assertSigV4(t, r, "bedrock")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"modelSummaries": []map[string]interface{}{
				{"modelId": "m1"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ai, err := awslib.NewAwsAi(awslib.AwsAiConfig{
		RegionName:         "us-east-1",
		AwsAccessKeyID:     "AKIA_TEST",
		AwsSecretAccessKey: "SECRET_TEST",
		EndpointURL:        srv.URL,
		HTTPClient:         srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAwsAi error: %v", err)
	}

	out, err := ai.InvokeModel(ctx, "anthropic.claude-v2", "prompt-text", 100, 0.5, map[string]interface{}{"top_k": 40})
	if err != nil || !strings.Contains(out, `"completion":"hello"`) {
		t.Fatalf("InvokeModel = (%q, %v)", out, err)
	}
	if _, err := ai.InvokeModel(ctx, "missing.model", "prompt", 100, 0.5, nil); err == nil {
		t.Fatal("expected error for missing Bedrock model")
	}

	models, err := ai.ListModels(ctx)
	if err != nil || len(models) != 1 || models[0]["modelId"] != "m1" {
		t.Fatalf("ListModels = (%v, %v)", models, err)
	}
}

func TestAwsBlobS3SigV4TransferAndSync(t *testing.T) {
	ctx := context.Background()
	s3Store := make(map[string][]byte)
	var putCount int

	mux := http.NewServeMux()
	mux.HandleFunc("/my-bucket", func(w http.ResponseWriter, r *http.Request) {
		assertSigV4(t, r, "s3")
		prefix := r.URL.Query().Get("prefix")
		var contents strings.Builder
		for k := range s3Store {
			if strings.HasPrefix(k, prefix) {
				fmt.Fprintf(&contents, "<Contents><Key>%s</Key></Contents>", k)
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><IsTruncated>false</IsTruncated>%s</ListBucketResult>`, contents.String())
	})
	mux.HandleFunc("/my-bucket/", func(w http.ResponseWriter, r *http.Request) {
		assertSigV4(t, r, "s3")
		key := strings.TrimPrefix(r.URL.Path, "/my-bucket/")
		switch r.Method {
		case http.MethodPut:
			putCount++
			b, _ := io.ReadAll(r.Body)
			s3Store[key] = b
			w.Header().Set("ETag", `"`+fileslib.CalculateLocalMD5(b)+`"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodHead:
			b, ok := s3Store[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("ETag", `"`+fileslib.CalculateLocalMD5(b)+`"`)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := s3Store[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s3Blob, err := awslib.NewAwsBlob(awslib.AwsBlobConfig{
		BucketName:         "my-bucket",
		RegionName:         "us-east-1",
		AwsAccessKeyID:     "AKIA_TEST",
		AwsSecretAccessKey: "SECRET_TEST",
		EndpointURL:        srv.URL,
		HTTPClient:         srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAwsBlob error: %v", err)
	}

	if got := s3Blob.FullKey("dir/x.txt"); got != "my-bucket/dir/x.txt" {
		t.Fatalf("FullKey = %q", got)
	}
	if got := s3Blob.FullKey("my-bucket/dir/x.txt"); got != "my-bucket/dir/x.txt" {
		t.Fatalf("FullKey prefixed = %q", got)
	}

	wrote, err := s3Blob.WriteIfDifferent(ctx, "dir/x.txt", []byte("customs_data"))
	if err != nil || !wrote || putCount != 1 {
		t.Fatalf("initial WriteIfDifferent = (%v, %v), putCount=%d", wrote, err, putCount)
	}
	wrote2, err := s3Blob.WriteIfDifferent(ctx, "dir/x.txt", []byte("customs_data"))
	if err != nil || wrote2 || putCount != 1 {
		t.Fatalf("second WriteIfDifferent = (%v, %v), putCount=%d", wrote2, err, putCount)
	}

	d, loaded, err := s3Blob.LoadOrStore(ctx, "dir/x.txt", func() ([]byte, error) {
		return []byte("customs_data"), nil
	})
	if err != nil || !loaded || string(d) != "customs_data" {
		t.Fatalf("LoadOrStore = (%q, %v, %v)", string(d), loaded, err)
	}

	// Transfer from real S3 HTTP server to LocalFS via FilesystemIndex
	localFS := fileslib.NewLocalFS()
	fss := fileslib.NewFilesystemIndex()
	fss.RegisterFilesystems(map[string]fileslib.FS{"aws": s3Blob, "local": localFS})

	awsFS, _ := fss.Filesystem("aws")
	lcFS, _ := fss.Filesystem("local")

	destDir := t.TempDir()
	if err := awsFS.Transfer(ctx, lcFS, "dir/*", destDir); err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	diskData, err := os.ReadFile(filepath.Join(destDir, "x.txt"))
	if err != nil || string(diskData) != "customs_data" {
		t.Fatalf("transferred disk file = %q (%v)", string(diskData), err)
	}
}

func TestAwsSecretsManagerSigV4WireProtocol(t *testing.T) {
	ctx := context.Background()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertSigV4(t, r, "secretsmanager")
		target := r.Header.Get("X-Amz-Target")
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch target {
		case "secretsmanager.GetSecretValue":
			if req["SecretId"] != "my-secret" {
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"__type": "ResourceNotFoundException", "Message": "not found"})
				return
			}
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			_ = json.NewEncoder(w).Encode(map[string]string{"SecretString": "top-secret"})
		case "secretsmanager.PutSecretValue":
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"ARN": "arn:aws:secretsmanager:us-east-1:123:secret:my-secret",
			})
		case "secretsmanager.ListSecrets":
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"SecretList": []map[string]interface{}{
					{"Name": "my-secret"},
				},
			})
		default:
			http.Error(w, "unknown target", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	sec, err := awslib.NewAwsSecrets(awslib.AwsSecretsConfig{
		RegionName:         "us-east-1",
		AwsAccessKeyID:     "AKIA_TEST",
		AwsSecretAccessKey: "SECRET_TEST",
		EndpointURL:        srv.URL,
		HTTPClient:         srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAwsSecrets error: %v", err)
	}

	val, err := sec.GetSecret(ctx, "my-secret")
	if err != nil || val != "top-secret" {
		t.Fatalf("GetSecret = (%q, %v)", val, err)
	}
	if _, err := sec.GetSecret(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing secret")
	}

	putRes, err := sec.PutSecret(ctx, "my-secret", "value")
	if err != nil || putRes["ARN"] != "arn:aws:secretsmanager:us-east-1:123:secret:my-secret" {
		t.Fatalf("PutSecret = (%v, %v)", putRes, err)
	}

	list, err := sec.ListSecrets(ctx, 10)
	if err != nil || len(list) != 1 || list[0]["Name"] != "my-secret" {
		t.Fatalf("ListSecrets = (%v, %v)", list, err)
	}
}

func TestAwsSqlConStrInspectAndSearch(t *testing.T) {
	ctx := context.Background()
	cfg := awslib.AwsSqlConfig{
		Driver:     "{ODBC Driver 18 for SQL Server}",
		Server:     "sql.example.com",
		Database:   "prod_db",
		Username:   "sa",
		Password:   "secret",
		RegionName: "us-east-1",
	}
	cs := cfg.ConStr()
	if !strings.Contains(cs, "Server=tcp:sql.example.com,1433;") || !strings.Contains(cs, "Database=prod_db;") || cfg.RegionName != "us-east-1" {
		t.Fatalf("unexpected ConStr / RegionName: %q / %q", cs, cfg.RegionName)
	}

	memDB, err := sqlitedriver.OpenMemory()
	if err != nil {
		t.Fatalf("OpenMemory error: %v", err)
	}
	defer memDB.Close()

	if _, err := memDB.ExecContext(ctx, "CREATE TABLE [items] ([Item] INTEGER PRIMARY KEY, [Description] TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE error: %v", err)
	}
	if _, err := memDB.ExecContext(ctx, "INSERT INTO [items] ([Item], [Description]) VALUES (1, 'A'), (2, 'B'), (3, 'C')"); err != nil {
		t.Fatalf("INSERT error: %v", err)
	}

	db := awslib.NewAwsSqlWithDB(memDB, cfg)

	tables, err := db.InspectTables(ctx, "")
	if err != nil || len(tables) != 1 || tables[0] != "items" {
		t.Fatalf("InspectTables = (%v, %v)", tables, err)
	}

	cols, err := db.InspectColumns(ctx, "items", "")
	if err != nil || len(cols) != 2 || cols[0].Name != "Item" || cols[1].Name != "Description" {
		t.Fatalf("InspectColumns = (%+v, %v)", cols, err)
	}

	meta, err := db.TableMetadata(ctx, "items", "")
	if err != nil || meta.Name != "items" {
		t.Fatalf("TableMetadata = (%+v, %v)", meta, err)
	}

	rowsAll, err := db.Search(ctx, sqllib.SearchOptions{Table: "items", PrimaryKey: "Item", OrderCol: "Item", LastItem: 0})
	if err != nil || len(rowsAll) != 3 {
		t.Fatalf("Search all = (%v, %v)", rowsAll, err)
	}

	rowsFiltered, err := db.Search(ctx, sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		OrderCol:   "Item",
		Filters:    map[string]interface{}{"Item": []int{2, 3}},
		LastItem:   1,
	})
	if err != nil || len(rowsFiltered) != 2 {
		t.Fatalf("Search list = (%v, %v)", rowsFiltered, err)
	}

	rowsAfter2, err := db.Search(ctx, sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		OrderCol:   "Item",
		LastItem:   2,
	})
	if err != nil || len(rowsAfter2) != 1 || rowsAfter2[0]["Description"] != "C" {
		t.Fatalf("Search last_item=2 = (%v, %v)", rowsAfter2, err)
	}
}
