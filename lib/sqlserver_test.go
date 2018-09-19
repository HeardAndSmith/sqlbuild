package lib

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

const TestPassword = "!SQLBUILD_TEST_2018"

// ResetSqlServer removes all of the files that sql server creates, effectively
// resetting the image to a clean state.
//
// Figured this out by running the mssql-server-linux image, executing some
// queries, and then calling `docker diff`. The files created are exclusively
// in /var/opt/mssql.
//
// This is sort of kludgy, since I think it's possible for queries to create
// files outside of this path (ie: CREATE DATABASE can save to some other
// path). But none of our tests do that, so I'm not sweating it.
func ResetSqlServer(t *testing.T) {
	t.Helper()
	if err := os.RemoveAll("/var/opt/mssql/"); err != nil {
		t.Fatalf("unexpected error resetting sql server: %s", err)
	}
}

func MustStartSqlServer(t *testing.T) *SqlServer {
	t.Helper()
	ResetSqlServer(t)
	sqlsrv, err := StartSqlServer(SqlServerEnv{
		SA_PASSWORD: TestPassword,
	})
	if err != nil {
		t.Fatalf("unexpected error starting sql server: %s", err)
	}
	sqlsrv.ExitOnUnexpectedShutdown()
	return sqlsrv
}

func MustShutdownSqlServer(t *testing.T, s *SqlServer) {
	t.Helper()
	if err := s.Shutdown(); err != nil {
		t.Fatalf("unexpected error shutting down sql server: %s", err)
	}
}

func TestStartSqlServer(t *testing.T) {
	ResetSqlServer(t)
	sqlsrv, err := StartSqlServer(SqlServerEnv{SA_PASSWORD: TestPassword})
	if err != nil {
		t.Fatal(err)
	}
	sqlsrv.ExitOnUnexpectedShutdown()

	db, err := OpenDatabase(TestPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var collation string
	err = db.QueryRow("SELECT DATABASEPROPERTYEX(N'master', 'Collation')").Scan(&collation)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("collation was %#v", collation)

	if err := sqlsrv.Shutdown(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond * 10)
}

func TestOnUnexpectedShutdown(t *testing.T) {
	ResetSqlServer(t)
	sqlsrv, err := StartSqlServer(SqlServerEnv{SA_PASSWORD: TestPassword})
	if err != nil {
		t.Fatal(err)
	}

	var callbackCalled uint32
	sqlsrv.OnUnexpectedShutdown(func(_ error) {
		atomic.StoreUint32(&callbackCalled, 1)
	})
	if err := sqlsrv.cmd.Process.Kill(); err != nil {
		t.Fatalf("error sending kill signal to sqlsrv process: %s", err)
	}
	sqlsrv.wait()
	time.Sleep(time.Millisecond * 10)
	if atomic.LoadUint32(&callbackCalled) != 1 {
		t.Fatal("callback was not called")
	}
}

func TestValidatePassword(t *testing.T) {
	valid := []string{
		"!SQLBUILD2018",
	}
	for _, pw := range valid {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("password %#v failed to validate: %s", pw, err)
		}
	}
	invalid := []string{
		"short",
	}
	for _, pw := range invalid {
		if err := ValidatePassword(pw); err == nil {
			t.Errorf("password %#v validated but should have failed", pw)
		}
	}
}

func TestPingErrorOutput(t *testing.T) {
	var err error
	err = sqlcmdPing(context.Background(), TestPassword)
	if err == nil {
		t.Fatal(err)
	}
	t.Logf("sql server not running:\n%s", err)

	sqlsrv := MustStartSqlServer(t)
	defer MustShutdownSqlServer(t, sqlsrv)

	err = sqlcmdPing(context.Background(), "!SQLBUILD2019")
	if err == nil {
		t.Fatal(err)
	}
	t.Logf("invalid password:\n%s", err)
}
