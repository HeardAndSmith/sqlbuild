package lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
)

func ValidatePassword(pw string) error {
	if len(pw) < 8 {
		return errors.New("Password too short")
	}
	if len(pw) > 128 {
		return errors.New("Password too long")
	}
	// TODO: There are more validations obviously, but waiting for the server to
	// come online ultimately reveals the issue anyway, so implementing them here
	// just makes the error more obvious. Will add if this package gets more use.
	return nil
}

type SqlServerEnv struct {
	SA_PASSWORD string
	MSSQL_PID   string
}

// Converts env to a []string like is expected by os/exec#Command.Env.
func (e SqlServerEnv) toCmdEnv() []string {
	env := []string{
		"ACCEPT_EULA=Y",
		"SA_PASSWORD=" + e.SA_PASSWORD,
	}
	if e.MSSQL_PID != "" {
		env = append(env, "MSSQL_PID="+e.MSSQL_PID)
	}
	return env
}

type SqlServer struct {
	cmd            *exec.Cmd
	env            SqlServerEnv
	exited         chan struct{}
	err            error
	output         *prefixSuffixSaver
	shutdownCalled uint32
}

func StartSqlServer(env SqlServerEnv) (*SqlServer, error) {
	output := &prefixSuffixSaver{N: 32 << 10}

	cmd := exec.Command("/opt/mssql/bin/sqlservr")
	cmd.Env = env.toCmdEnv()
	cmd.Stdout = output
	cmd.Stderr = output

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("Failed to start sqlservr: %s", err)
	}

	s := &SqlServer{
		cmd:    cmd,
		env:    env,
		exited: make(chan struct{}),
		output: output,
	}

	// Start a goroutine that waits for the process to exit.
	go s.waitroutine()

	if err := waitForSqlServerToComeOnline(s.exited, time.Second*15, env.SA_PASSWORD); err != nil {

		// Check whether the sql server process has exited. If it has, the issue
		// was likely with startup, so ignore the wait error.
		if s.hasExited() {
			return nil, fmt.Errorf(
				"---------- SQLSERVER OUTPUT ----------\n"+
					"%s\n"+
					"--------------------------------------\n"+
					"Failed to start sqlservr: process exited after %s.\n"+
					"Process stdout/stderr printed above.\n"+
					"ExitError: %s\n",
				outputToString(s.output.Bytes()),
				time.Since(start),
				errOrNil(s.err),
			)
		}

		// Otherwise, we need to shutdown the process and then return an error,
		// providing as much information as possible to debug _why_ connection
		// wasn't possible.
		after := time.Since(start)
		shutdownErr := s.Shutdown()
		return nil, fmt.Errorf(
			"---------- SQLSERVER OUTPUT ----------\n"+
				"%s\n"+
				"--------------------------------------\n"+
				"Failed to start sqlservr: connection failed after %s.\n"+
				"Process stdout/stderr printed above.\n"+
				"ShutdownError: %s\n"+
				"ConnectError: %s",
			outputToString(s.output.Bytes()),
			after,
			errOrNil(shutdownErr),
			err,
		)
	}
	return s, nil
}

// waitroutine waits for the command to complete, saves the error, and then
// closes the "exited" channel.
func (s *SqlServer) waitroutine() {
	s.err = s.cmd.Wait()
	close(s.exited)
}

func (s *SqlServer) wait() error {
	<-s.exited
	return s.err
}

func (s *SqlServer) hasExited() bool {
	return doneIsClosed(s.exited)
}

// OnUnexpectedShutdown installs a callback that will be called if the server
// process exits before Shutdown is called.
func (s *SqlServer) OnUnexpectedShutdown(callback func(err error)) {
	if callback == nil {
		panic("OnUnexpectedShutdown passed nil callback")
	}
	go func() {
		err := s.wait()

		// The process exited, but we never sent a shutdown signal. Log a fatal
		// error, trying to capture as much context as possible.
		expected := atomic.LoadUint32(&s.shutdownCalled) == 1
		if !expected {
			callback(fmt.Errorf(
				"---------- SQLSERVER OUTPUT ----------\n"+
					"%s\n"+
					"--------------------------------------\n"+
					"sqlserver process exited unexpectedly!!!\n"+
					"Error: %s",
				outputToString(s.output.Bytes()),
				errOrNil(err),
			))
		}
	}()
}

// ExitOnUnexpectedShutdown exits when the sql server process exits before
// Shutdown is called. Because all of the code we use generally expects for
// sqlserver to run continuously until Shutdown is called, this needs to be
// called right after the server is created.
func (s *SqlServer) ExitOnUnexpectedShutdown() {
	s.OnUnexpectedShutdown(func(err error) {
		log.Fatalf("%s\nThis is a fatal error, exiting now.",
			err.Error(),
		)
	})
}

func (s *SqlServer) Shutdown() error {
	if s.hasExited() {
		return s.err
	}
	atomic.StoreUint32(&s.shutdownCalled, 1)
	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("Sending SIGTERM to sqlservr failed unexpectedly: %s", err)
	}
	return s.wait()
}

// Takes stdout/stderr style output, trims trailing whitespace, and returns a
// string.
func outputToString(b []byte) string {
	return string(bytes.TrimRightFunc(b, unicode.IsSpace))
}

var ErrCanceled = context.Canceled

func waitForSqlServerToComeOnline(
	done <-chan struct{},
	timeout time.Duration,
	password string,
) error {
	return pingLoop(done, timeout, password)
}

// doneIsClosed synchronously returns whether a done channel is closed.
func doneIsClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func errOrNil(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func pingLoop(
	done <-chan struct{},
	timeout time.Duration,
	password string,
) error {
	deadline := time.Now().Add(timeout)

	ping := func() error {
		// Don't use the loop deadline to cancel the ping command: I would rather
		// it exit naturally and provide a decent error message than be cancelled
		// via a signal and return something confusing, especially since the last
		// error is the one that's ultimately returned.
		//
		// However, we can still cancel it on done since that means we're no longer
		// interested in the result at all.
		return sqlcmdPing(doneCtx{done}, password)
	}

	var err error
	if err = ping(); err == nil {
		return nil
	}

	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	for {
		select {
		case <-poll.C:
			if err = ping(); err == nil {
				return nil
			} else if time.Now().After(deadline) {
				return err
			}
		case <-done:
			return ErrCanceled
		}
	}
}

type PingError struct {
	Err    error
	Output []byte
}

func (e *PingError) Error() string {
	return fmt.Sprintf(
		"sqlcmd ping failed with error: %s\n"+
			"---------- SQLCMD OUTPUT ----------\n"+
			"%s\n"+
			"-----------------------------------",
		e.Err.Error(),
		outputToString(e.Output),
	)
}

func sqlcmdPing(ctx context.Context, password string) error {
	// Add long timeout to handle sqlcmd blocking unexpectedly. The query timeout
	// argument should shut it down before this ever has to fire, but better safe
	// than sorry.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	// sqlcmd documentation:
	// https://docs.microsoft.com/en-us/sql/tools/sqlcmd-utility?view=sql-server-2017#syntax
	cmd := exec.CommandContext(
		ctx,
		"/opt/mssql-tools/bin/sqlcmd",
		"-S", "localhost",
		"-U", "SA",
		"-P", password,
		"-l", "5", // login_timeout
		"-t", "15", // query_timeout
		"-h", "-1", // dont print headers
		// We don't actually check the output, so not sure how useful this is, but
		// the query is basically from here:
		// https://dba.stackexchange.com/a/198912/79716
		"-Q", "SET NOCOUNT ON; SELECT CASE WHEN DATABASEPROPERTYEX(N'master', 'Collation') IS NULL THEN 'FALSE' ELSE 'TRUE' END;",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &PingError{
			Err:    err,
			Output: output,
		}
	}
	return nil
}

type doneCtx struct {
	done <-chan struct{}
}

func (d doneCtx) Err() error {
	select {
	case <-d.done:
		return context.Canceled
	default:
		return nil
	}
}
func (d doneCtx) Done() <-chan struct{}                     { return d.done }
func (d doneCtx) Deadline() (deadline time.Time, ok bool)   { return }
func (d doneCtx) Value(key interface{}) (value interface{}) { return }
