package testdb_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"smartcampaign/internal/testdb"
)

// runChild runs this test binary again, with only TestNoDatabase, without the
// test database settings and with extra variables.
func runChild(t *testing.T, extra ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNoDatabase$", "-test.v", "-test.count=1")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TEST_DATABASE_URL=") && !strings.HasPrefix(kv, "SKIP_DB_TESTS=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, append([]string{"TESTDB_CHILD=1"}, extra...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestNoDatabase is what the child runs: a test that needs a database.
func TestNoDatabase(t *testing.T) {
	if os.Getenv("TESTDB_CHILD") != "1" {
		t.Skip("run by the tests below")
	}
	testdb.NewEmpty(t)
}

// `go test` on a machine without the test database must not look green: the
// database tests fail and say how to run them.
func TestWithoutTheDatabaseTheTestsFailLoudly(t *testing.T) {
	out, err := runChild(t)
	if err == nil {
		t.Fatalf("a database test passed without a database:\n%s", out)
	}
	if !strings.Contains(out, "TEST_DATABASE_URL tanımlı değil; testleri 'make test' ile çalıştırın.") {
		t.Fatalf("the failure must say how to run the tests:\n%s", out)
	}
}

// SKIP_DB_TESTS=1 is the explicit way to run only the tests without a database.
func TestSkipDBTestsSkipsThem(t *testing.T) {
	out, err := runChild(t, "SKIP_DB_TESTS=1")
	if err != nil || !strings.Contains(out, "--- SKIP: TestNoDatabase") {
		t.Fatalf("SKIP_DB_TESTS=1 must skip the database tests (err %v):\n%s", err, out)
	}
}
