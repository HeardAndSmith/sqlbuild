package lib

import (
	"context"
	"database/sql"
	"github.com/denisenkom/go-mssqldb"
	"net/url"
)

func buildDSN(password string) string {
	u := &url.URL{
		Scheme: "sqlserver",
		User:   url.UserPassword("SA", password),
		Host:   "localhost",
	}
	return u.String()
}

func OpenDatabase(password string) (*sql.DB, error) {
	dsn := buildDSN(password)
	return sql.Open("sqlserver", dsn)
}

func NewConn(password string) (*SqlConn, error) {
	dsn := buildDSN(password)
	connector, err := mssql.NewConnector(dsn)
	if err != nil {
		return nil, err
	}
	iconn, err := connector.Connect(context.Background())
	if err != nil {
		return nil, err
	}
	return &SqlConn{iconn.(*mssql.Conn)}, nil
}

type SqlConn struct {
	conn *mssql.Conn
}

func (c *SqlConn) Close() error {
	return c.conn.Close()
}

func (c *SqlConn) ResetSession() error {
	return c.conn.ResetSession(context.Background())
}

func (c *SqlConn) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

func (c *SqlConn) Exec(query string) error {
	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(nil)
	return err
}
