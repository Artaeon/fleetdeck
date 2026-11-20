package fleet

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportReaderRouteIsSeparateFromFleetSnapshot(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	snapshot := Snapshot{Schema: 1, NodeID: "node-a", GeneratedAt: now, Monitoring: Checks{Status: "unknown"}, Security: Security{Status: "unknown"}}
	b, _ := json.Marshal(snapshot)
	if err := os.WriteFile(snapshotPath, b, 0600); err != nil {
		t.Fatal(err)
	}
	statusPath := filepath.Join(dir, "status.json")
	want := ReportRuns{Daily: ReportRun{now, 0}, Weekly: ReportRun{now, 1}}
	b, _ = json.Marshal(want)
	if err := os.WriteFile(statusPath, b, 0640); err != nil {
		t.Fatal(err)
	}
	operator, reader := &x509.Certificate{Raw: []byte("operator")}, &x509.Certificate{Raw: []byte("reader")}
	pins := func(c *x509.Certificate) map[[32]byte]bool { return map[[32]byte]bool{sha256.Sum256(c.Raw): true} }
	handler := WithReportStatus(Agent("node-a", snapshotPath, pins(operator)), statusPath, pins(reader))
	for _, tc := range []struct {
		peer         *x509.Certificate
		path, method string
		code         int
	}{
		{operator, "/v1/snapshot", "GET", 200}, {reader, "/v1/report-status", "GET", 200},
		{reader, "/v1/snapshot", "GET", 401}, {operator, "/v1/report-status", "GET", 401},
		{nil, "/v1/report-status", "GET", 401}, {reader, "/v1/report-status", "POST", 405},
		{reader, "/v1/report-status?details=true", "GET", 404},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.peer != nil {
			req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{tc.peer}}, PeerCertificates: []*x509.Certificate{tc.peer}}
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.code {
			t.Fatalf("%s %s got %d want %d", tc.method, tc.path, res.Code, tc.code)
		}
		if tc.code == 200 && tc.path == "/v1/report-status" && (strings.TrimSpace(res.Body.String()) != string(b) || res.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("unexpected disclosure", res.Body.String())
		}
	}
	if err := os.Chmod(statusPath, 0664); err != nil {
		t.Fatal(err)
	}
	if _, err := readReportRuns(statusPath, now); err == nil {
		t.Fatal("accepted group-writable or public file")
	}
}

func TestReportStatusRequiresBothTimesAndExplicitCounts(t *testing.T) {
	now := time.Now().UTC()
	good := `{"last_evaluation":"` + now.Format(time.RFC3339Nano) + `","pending":0}`
	for _, s := range []string{"", `{}`, `{"daily":` + good + `}`, `{"daily":{"pending":0},"weekly":` + good + `}`,
		`{"daily":` + good + `,"weekly":` + strings.Replace(good, `,"pending":0`, "", 1) + `}`,
		`{"daily":` + good + `,"weekly":` + strings.Replace(good, `"pending":0`, `"pending":-1`, 1) + `}`,
		`{"daily":` + good + `,"weekly":` + good + `,"raw":"private"}`, `{"daily":` + good + `,"weekly":` + good + `}{}`, strings.Repeat(" ", 4097)} {
		if _, err := DecodeReportRuns(strings.NewReader(s), now); err == nil {
			t.Fatalf("invalid status accepted: %.90s", s)
		}
	}
	if _, err := DecodeReportRuns(strings.NewReader(`{"daily":`+good+`,"weekly":`+good+`}`), now.Add(-time.Hour)); err == nil {
		t.Fatal("accepted a future clock")
	}
}
