package storage

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgmock"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/rs/zerolog"
)

// --- Environment & Database Setup Helpers ---

func loadEnv() {
	if os.Getenv("DATABASE_URL") != "" {
		return
	}
	candidates := []string{".env", "../.env", "../../.env", "../../../.env"}
	for _, c := range candidates {
		absPath, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		f, err := os.Open(absPath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if os.Getenv(k) == "" {
					os.Setenv(k, v)
				}
			}
		}
		f.Close()
		if os.Getenv("DATABASE_URL") != "" {
			break
		}
	}
}

func ensureSchema(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	ddl := fmt.Sprintf(`
CREATE SCHEMA IF NOT EXISTS %s;

CREATE TABLE IF NOT EXISTS %s.groups (
    id bigint NOT NULL PRIMARY KEY,
    version bigint NOT NULL DEFAULT 0,
    created timestamp with time zone NOT NULL DEFAULT NOW(),
    modified timestamp with time zone NOT NULL DEFAULT NOW(),
    data text,
    deleted boolean NOT NULL DEFAULT false,
    itemversion bigint NOT NULL DEFAULT 0,
    collectionversion bigint NOT NULL DEFAULT 0,
    tagversion bigint NOT NULL DEFAULT 0,
    gitlab timestamp with time zone
);

CREATE TABLE IF NOT EXISTS %s.syncgroups (
    id bigint NOT NULL,
    active boolean DEFAULT true NOT NULL,
    direction varchar(32) DEFAULT 'none' NOT NULL,
    tags boolean DEFAULT false NOT NULL,
    CONSTRAINT syncgroups_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS %s.collections (
    key varchar(32) NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    library bigint NOT NULL,
    sync varchar(32) NOT NULL,
    data text,
    meta text,
    deleted boolean DEFAULT false NOT NULL,
    modified timestamp with time zone DEFAULT NOW(),
    gitlab timestamp with time zone,
    PRIMARY KEY (library, key)
);

CREATE TABLE IF NOT EXISTS %s.items (
    key varchar(32) NOT NULL,
    version bigint DEFAULT 0 NOT NULL,
    library bigint NOT NULL,
    sync varchar(32) NOT NULL,
    data text,
    meta text,
    oldid varchar(128),
    trashed boolean DEFAULT false NOT NULL,
    deleted boolean DEFAULT false NOT NULL,
    md5 varchar(64),
    modified timestamp with time zone DEFAULT NOW(),
    gitlab timestamp with time zone,
    PRIMARY KEY (library, key),
    CONSTRAINT items_oldid_constraint UNIQUE (library, oldid)
);

CREATE TABLE IF NOT EXISTS %s.tags (
    tag varchar(255) NOT NULL,
    meta text,
    library bigint NOT NULL,
    CONSTRAINT pk_tags PRIMARY KEY (library, tag)
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_matviews WHERE schemaname = '%s' AND matviewname = 'collection_name_hier') THEN
        EXECUTE 'CREATE MATERIALIZED VIEW %s.collection_name_hier AS SELECT key, library, (data::json->>''name'') AS name, (data::json->>''parentCollection'') AS parent FROM %s.collections';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_matviews WHERE schemaname = '%s' AND matviewname = 'item_type_hier') THEN
        EXECUTE 'CREATE MATERIALIZED VIEW %s.item_type_hier AS SELECT key, library, (data::json->>''itemType'') AS type, (data::json->>''parentItem'') AS parent FROM %s.items';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION %s.refresh_item_type_hier() RETURNS void AS $$
BEGIN
    REFRESH MATERIALIZED VIEW %s.item_type_hier WITH DATA;
END;
$$ LANGUAGE plpgsql;
`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)

	_, err := pool.Exec(ctx, ddl)
	return err
}

func getTestStorage(t *testing.T) (*Storage, int64, func()) {
	t.Helper()
	loadEnv()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; skipping integration tests")
		return nil, 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Skipf("cannot parse connection url (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	schema := "public"
	if s := os.Getenv("DATABASE_SCHEMA"); s != "" {
		schema = s
	}

	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = make(map[string]string)
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["search_path"]; !ok {
		cfg.ConnConfig.RuntimeParams["search_path"] = fmt.Sprintf("%s, public", schema)
	}

	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf("SET search_path = %s, public", schema))
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Skipf("cannot create connection pool (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("cannot ping database (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	if err := ensureSchema(ctx, pool, schema); err != nil {
		pool.Close()
		t.Skipf("cannot ensure schema (%v); skipping integration tests", err)
		return nil, 0, nil
	}

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	st := NewStorage(pool, schema, true, &zlog)

	testGroupID := int64(88880000 + (time.Now().UnixNano() % 10000))

	cleanupGroup := func() {
		bgCtx := context.Background()
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.tags WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.items WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.collections WHERE library=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.syncgroups WHERE id=$1", schema), testGroupID)
		_, _ = pool.Exec(bgCtx, fmt.Sprintf("DELETE FROM %s.groups WHERE id=$1", schema), testGroupID)
	}
	cleanupGroup()

	cleanup := func() {
		cleanupGroup()
		pool.Close()
	}

	return st, testGroupID, cleanup
}

// --- Wire Protocol Binary Encoders & Mock Server ---

func encodeInt8(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func encodeBool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

func encodeText(s string) []byte {
	return []byte(s)
}

func encodeTimestamp(t time.Time) []byte {
	pgEpoch := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	micro := t.UTC().Sub(pgEpoch).Microseconds()
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(micro))
	return b
}

func startMockServer(t *testing.T, script *pgmock.Script) (*Storage, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on tcp: %v", err)
	}

	serverErrChan := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverErrChan <- err
			return
		}
		defer conn.Close()
		backend := pgproto3.NewBackend(pgproto3.NewChunkReader(conn), conn)
		serverErrChan <- script.Run(backend)
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	dbURL := fmt.Sprintf("postgres://mockuser:mockpass@127.0.0.1:%d/mockdb?sslmode=disable&default_query_exec_mode=describe_exec", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		_ = ln.Close()
		t.Fatalf("failed to create connection pool to mock: %v", err)
	}

	zlog := zerolog.Nop()
	st := NewStorage(pool, "public", true, &zlog)

	cleanup := func() {
		pool.Close()
		_ = ln.Close()
		select {
		case err := <-serverErrChan:
			if err != nil && !errors.Is(err, net.ErrClosed) && !strings.Contains(err.Error(), "use of closed network connection") && !errors.Is(err, io.EOF) {
				t.Errorf("mock server error: %v", err)
			}
		case <-time.After(2 * time.Second):
		}
	}

	return st, cleanup
}

// --- Test Data Fixtures & Factories ---

func sampleGroupData(id int64, name string) *model.GroupData {
	return &model.GroupData{
		Id:          id,
		Version:     1,
		Name:        name,
		Description: "Sample group description for testing",
		Type:        "Private",
		Owner:       12345,
	}
}

func sampleCollectionData(key string, name string, parent string) *model.CollectionData {
	return &model.CollectionData{
		Key:              key,
		Version:          1,
		Name:             name,
		ParentCollection: model.Parent(parent),
	}
}

func sampleItemData(key string, title string, itemType string) *model.ItemGeneric {
	item := &model.ItemGeneric{}
	item.Key = key
	item.Version = 1
	item.ItemType = itemType
	item.Title = title
	item.Creators = []model.ItemDataPerson{
		{
			CreatorType: "author",
			FirstName:   "Test",
			LastName:    "Author",
		},
	}
	return item
}

func sampleTag(name string) model.Tag {
	return model.Tag{
		Tag: name,
		Meta: &model.TagMeta{
			Type: 0,
		},
	}
}
