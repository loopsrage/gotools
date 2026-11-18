package azurelib_test

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loopsrage/gotools/lib/azurelib"
	"github.com/loopsrage/gotools/lib/fileslib"
	"github.com/loopsrage/gotools/lib/sqllib"
	"github.com/loopsrage/gotools/tests/internal/sqlitedriver"
)

func TestAzureAiSDKChatAndEmbed(t *testing.T) {
	ctx := context.Background()

	if _, err := azurelib.NewAzureAi(azurelib.AzureAiConfig{}); err == nil {
		t.Fatal("expected NewAzureAi to fail when AzureEndpoint is empty")
	}

	var seenDeployment string
	mux := http.NewServeMux()
	mux.HandleFunc("/openai/deployments/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "secret-key" {
			http.Error(w, "missing api-key", http.StatusUnauthorized)
			return
		}
		trimmed := strings.TrimPrefix(r.URL.Path, "/openai/deployments/")
		parts := strings.SplitN(trimmed, "/", 2)
		seenDeployment = parts[0]
		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      "chatcmpl-123",
				"object":  "chat.completion",
				"created": 1700000000,
				"model":   seenDeployment,
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"finish_reason": "stop",
						"message": map[string]string{
							"role":    "assistant",
							"content": "hello world",
						},
					},
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"model":  seenDeployment,
				"usage":  map[string]int{"prompt_tokens": 2, "total_tokens": 2},
				"data": []map[string]interface{}{
					{
						"object":    "embedding",
						"index":     0,
						"embedding": []float64{0.1, 0.2, 0.3},
					},
				},
			})
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ai, err := azurelib.NewAzureAi(azurelib.AzureAiConfig{
		AzureEndpoint:   srv.URL,
		APIKey:          "secret-key",
		APIVersion:      "2024-02-01",
		AzureDeployment: "my-deploy",
		HTTPClient:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAzureAi error: %v", err)
	}

	reply, err := ai.Chat(ctx, []azurelib.ChatMessage{{Role: "user", Content: "hi"}}, "gpt-4", 0.5, 100)
	if err != nil || reply != "hello world" || seenDeployment != "gpt-4" {
		t.Fatalf("Chat explicit = (%q, %v), seenDeployment=%q", reply, err, seenDeployment)
	}

	reply2, err := ai.Chat(ctx, []azurelib.ChatMessage{{Role: "user", Content: "hi"}}, "", 0.7, 512)
	if err != nil || reply2 != "hello world" || seenDeployment != "my-deploy" {
		t.Fatalf("Chat fallback = (%q, %v), seenDeployment=%q", reply2, err, seenDeployment)
	}

	vec, err := ai.Embed(ctx, "hello world", "embed-1")
	if err != nil || len(vec) != 3 || vec[0] != 0.1 || vec[2] != 0.3 {
		t.Fatalf("Embed = (%v, %v)", vec, err)
	}
}

func TestAzureMD5HelpersAndAzBlobSDK(t *testing.T) {
	ctx := context.Background()
	data := []byte("tariff_payload_123")
	sum := md5.Sum(data)
	expectedHex := strings.ToLower(hex.EncodeToString(sum[:]))

	if got := azurelib.CalculateLocalMD5(data); got != expectedHex {
		t.Fatalf("CalculateLocalMD5 = %q, want %q", got, expectedHex)
	}
	if got := azurelib.NormalizeMD5(""); got != "" {
		t.Fatalf("NormalizeMD5('') = %q, want ''", got)
	}
	if got := azurelib.NormalizeMD5Bytes(sum[:]); got != expectedHex {
		t.Fatalf("NormalizeMD5Bytes = %q, want %q", got, expectedHex)
	}
	if got := azurelib.NormalizeMD5(`"` + strings.ToUpper(expectedHex) + `"`); got != expectedHex {
		t.Fatalf("NormalizeMD5 quoted = %q", got)
	}
	if got := azurelib.NormalizeMD5(base64.StdEncoding.EncodeToString(sum[:])); got != expectedHex {
		t.Fatalf("NormalizeMD5 base64 = %q", got)
	}
	if got := azurelib.NormalizeMD5("not-a-md5"); got != "not-a-md5" {
		t.Fatalf("NormalizeMD5 unknown = %q", got)
	}

	blobs := make(map[string][]byte)
	var putCalls int

	mux := http.NewServeMux()
	mux.HandleFunc("/tariffs", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "SharedKey testacct:") {
			http.Error(w, "missing SharedKey signature", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("comp") == "list" {
			prefix := r.URL.Query().Get("prefix")
			var xmlItems strings.Builder
			for k := range blobs {
				if strings.HasPrefix(k, prefix) {
					fmt.Fprintf(&xmlItems, "<Blob><Name>%s</Name><Properties/></Blob>", k)
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults><Blobs>%s</Blobs><NextMarker/></EnumerationResults>`, xmlItems.String())
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/tariffs/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "SharedKey testacct:") {
			http.Error(w, "missing SharedKey signature", http.StatusUnauthorized)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/tariffs/")
		switch r.Method {
		case http.MethodPut:
			putCalls++
			b, _ := io.ReadAll(r.Body)
			blobs[key] = b
			s := md5.Sum(b)
			w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(s[:]))
			w.Header().Set("ETag", `"`+fileslib.CalculateLocalMD5(b)+`"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			b, ok := blobs[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			s := md5.Sum(b)
			w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(s[:]))
			w.Header().Set("ETag", `"`+fileslib.CalculateLocalMD5(b)+`"`)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			b, ok := blobs[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			s := md5.Sum(b)
			w.Header().Set("Content-MD5", base64.StdEncoding.EncodeToString(s[:]))
			w.Header().Set("ETag", `"`+fileslib.CalculateLocalMD5(b)+`"`)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(b)))
			_, _ = w.Write(b)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	acctKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	az, err := azurelib.NewAzureBlob(azurelib.AzureBlobConfig{
		AccountName:   "testacct",
		AccountKey:    acctKey,
		ContainerName: "tariffs",
		EndpointURL:   srv.URL,
		HTTPClient:    srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAzureBlob error: %v", err)
	}

	wrote, err := az.WriteIfDifferent(ctx, "dir/item.txt", []byte("customs_data"))
	if err != nil || !wrote || putCalls != 1 {
		t.Fatalf("initial WriteIfDifferent = (%v, %v), putCalls=%d", wrote, err, putCalls)
	}
	wroteAgain, err := az.WriteIfDifferent(ctx, "dir/item.txt", []byte("customs_data"))
	if err != nil || wroteAgain || putCalls != 1 {
		t.Fatalf("second WriteIfDifferent = (%v, %v), putCalls=%d", wroteAgain, err, putCalls)
	}

	got, loaded, err := az.LoadOrStore(ctx, "dir/item.txt", func() ([]byte, error) {
		return []byte("customs_data"), nil
	})
	if err != nil || !loaded || string(got) != "customs_data" {
		t.Fatalf("LoadOrStore = (%q, %v, %v)", string(got), loaded, err)
	}

	localFS := fileslib.NewLocalFS()
	fss := fileslib.NewFilesystemIndex()
	fss.RegisterFilesystems(map[string]fileslib.FS{"azure": az, "local": localFS})

	azFS, _ := fss.Filesystem("azure")
	lcFS, _ := fss.Filesystem("local")

	destDir := t.TempDir()
	if err := azFS.Transfer(ctx, lcFS, "dir/*", destDir); err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	diskData, err := os.ReadFile(filepath.Join(destDir, "item.txt"))
	if err != nil || string(diskData) != "customs_data" {
		t.Fatalf("transferred disk file = %q (%v)", string(diskData), err)
	}
}

func TestAzureSecretsSDKKeyVault(t *testing.T) {
	ctx := context.Background()

	var srvURL string
	requireVaultBearer := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer kv-token" {
			w.Header().Set("WWW-Authenticate", `Bearer authorization="https://login.microsoftonline.com/common", resource="https://vault.azure.net"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		return true
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/secrets/my-secret", func(w http.ResponseWriter, r *http.Request) {
		if !requireVaultBearer(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    srvURL + "/secrets/my-secret/v1",
				"value": "value",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    srvURL + "/secrets/my-secret/v1",
			"value": "top-secret",
		})
	})
	mux.HandleFunc("/secrets/my-secret/", func(w http.ResponseWriter, r *http.Request) {
		if !requireVaultBearer(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    srvURL + "/secrets/my-secret/v1",
			"value": "top-secret",
		})
	})
	mux.HandleFunc("/secrets", func(w http.ResponseWriter, r *http.Request) {
		if !requireVaultBearer(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"value": []map[string]interface{}{
					{"id": srvURL + "/secrets/s2", "attributes": map[string]bool{"enabled": true}},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"value": []map[string]interface{}{
				{"id": srvURL + "/secrets/s1", "attributes": map[string]bool{"enabled": true}},
			},
			"nextLink": srvURL + "/secrets?page=2",
		})
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	kv, err := azurelib.NewAzureSecrets(azurelib.AzureSecretsConfig{
		VaultURL:   srv.URL,
		Credential: azurelib.StaticTokenCredential("kv-token"),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("NewAzureSecrets error: %v", err)
	}

	val, err := kv.GetSecret(ctx, "my-secret")
	if err != nil || val != "top-secret" {
		t.Fatalf("GetSecret = (%q, %v)", val, err)
	}
	if _, err := kv.GetSecret(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing secret")
	}

	putRes, err := kv.PutSecret(ctx, "my-secret", "value")
	if err != nil || putRes["name"] != "my-secret" || putRes["version"] != "v1" {
		t.Fatalf("PutSecret = (%v, %v), want name=my-secret version=v1", putRes, err)
	}

	props, err := kv.ListSecrets(ctx, 10)
	if err != nil || len(props) != 2 || props[0].Name != "s1" || props[1].Name != "s2" {
		t.Fatalf("ListSecrets paginated = (%+v, %v)", props, err)
	}
}

func TestAzureSqlConStrInspectAndSearch(t *testing.T) {
	ctx := context.Background()
	cfg := azurelib.AzureSqlConfig{
		Driver:   "{ODBC Driver 18 for SQL Server}",
		Server:   "sql.example.com",
		Database: "prod_db",
		Username: "sa",
		Password: "secret",
	}
	cs := cfg.ConStr()
	for _, wantSub := range []string{
		"Driver={ODBC Driver 18 for SQL Server};",
		"Server=tcp:sql.example.com,1433;",
		"Database=prod_db;",
		"UID=sa;",
		"PWD=secret;",
		"TrustServerCertificate=yes;",
	} {
		if !strings.Contains(cs, wantSub) {
			t.Fatalf("ConStr %q missing %q", cs, wantSub)
		}
	}

	// Verify go-mssqldb ("sqlserver") default driver registration opens cleanly
	sqlPool, err := azurelib.NewAzureSql("", "", cfg)
	if err != nil {
		t.Fatalf("NewAzureSql default sqlserver open failed: %v", err)
	}
	_ = sqlPool.Close()

	mssqlOnly := azurelib.NewAzureSqlWithDB(nil, cfg)
	tsql, tsqlArgs, err := mssqlOnly.BuildSearchQuery(sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		Columns:    "*",
		OrderCol:   "Item",
		Filters:    map[string]interface{}{"Description": "B"},
		Limit:      sqllib.IntPtr(50),
		LastItem:   0,
	})
	if err != nil {
		t.Fatalf("BuildSearchQuery(mssql) error: %v", err)
	}
	wantTSQL := "SELECT TOP 50 * FROM [items] AS base WHERE base.[Item] > @p1 AND base.[Description] = @p2 ORDER BY [Item]"
	if tsql != wantTSQL || len(tsqlArgs) != 2 {
		t.Fatalf("BuildSearchQuery(mssql) = %q (%v), want %q", tsql, tsqlArgs, wantTSQL)
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

	db := azurelib.NewAzureSqlWithDB(memDB, cfg)
	tables, err := db.InspectTables(ctx, "")
	if err != nil || len(tables) != 1 || tables[0] != "items" {
		t.Fatalf("InspectTables = (%v, %v)", tables, err)
	}

	rowsScalar, err := db.Search(ctx, sqllib.SearchOptions{
		Table:      "items",
		PrimaryKey: "Item",
		Filters:    map[string]interface{}{"Description": "B"},
		LastItem:   0,
	})
	if err != nil || len(rowsScalar) != 1 || rowsScalar[0]["Item"] != int64(2) {
		t.Fatalf("Search scalar = (%v, %v)", rowsScalar, err)
	}
}
