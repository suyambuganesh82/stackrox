// Code generated by pg-bindings generator. DO NOT EDIT.

package postgres

import (
	"context"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stackrox/rox/central/metrics"
	pkgSchema "github.com/stackrox/rox/central/postgres/schema"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/logging"
	ops "github.com/stackrox/rox/pkg/metrics"
	"github.com/stackrox/rox/pkg/postgres/pgutils"
	"github.com/stackrox/rox/pkg/search"
	"github.com/stackrox/rox/pkg/search/postgres"
)

const (
	baseTable = "singlekey"

	getStmt     = "SELECT serialized FROM singlekey WHERE Key = $1"
	walkStmt    = "SELECT serialized FROM singlekey"
	getManyStmt = "SELECT serialized FROM singlekey WHERE Key = ANY($1::text[])"

	batchAfter = 100

	// using copyFrom, we may not even want to batch.  It would probably be simpler
	// to deal with failures if we just sent it all.  Something to think about as we
	// proceed and move into more e2e and larger performance testing
	batchSize = 10000
)

var (
	log    = logging.LoggerForModule()
	schema = pkgSchema.SinglekeySchema
)

type Store interface {
	Count(ctx context.Context) (int, error)
	Exists(ctx context.Context, key string) (bool, error)
	Get(ctx context.Context, key string) (*storage.TestSingleKeyStruct, bool, error)
	GetAll(ctx context.Context) ([]*storage.TestSingleKeyStruct, error)
	Upsert(ctx context.Context, obj *storage.TestSingleKeyStruct) error
	UpsertMany(ctx context.Context, objs []*storage.TestSingleKeyStruct) error
	Delete(ctx context.Context, key string) error
	GetIDs(ctx context.Context) ([]string, error)
	GetMany(ctx context.Context, ids []string) ([]*storage.TestSingleKeyStruct, []int, error)
	DeleteMany(ctx context.Context, ids []string) error

	Walk(ctx context.Context, fn func(obj *storage.TestSingleKeyStruct) error) error

	AckKeysIndexed(ctx context.Context, keys ...string) error
	GetKeysToIndex(ctx context.Context) ([]string, error)
}

type storeImpl struct {
	db *pgxpool.Pool
}

// New returns a new Store instance using the provided sql instance.
func New(ctx context.Context, db *pgxpool.Pool) Store {
	pgutils.CreateTable(ctx, db, pkgSchema.CreateTableSinglekeyStmt)

	return &storeImpl{
		db: db,
	}
}

func insertIntoSinglekey(ctx context.Context, tx pgx.Tx, obj *storage.TestSingleKeyStruct) error {

	serialized, marshalErr := obj.Marshal()
	if marshalErr != nil {
		return marshalErr
	}

	values := []interface{}{
		// parent primary keys start
		obj.GetKey(),
		obj.GetName(),
		obj.GetStringSlice(),
		obj.GetBool(),
		obj.GetUint64(),
		obj.GetInt64(),
		obj.GetFloat(),
		obj.GetLabels(),
		pgutils.NilOrTime(obj.GetTimestamp()),
		obj.GetEnum(),
		obj.GetEnums(),
		serialized,
	}

	finalStr := "INSERT INTO singlekey (Key, Name, StringSlice, Bool, Uint64, Int64, Float, Labels, Timestamp, Enum, Enums, serialized) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT(Key) DO UPDATE SET Key = EXCLUDED.Key, Name = EXCLUDED.Name, StringSlice = EXCLUDED.StringSlice, Bool = EXCLUDED.Bool, Uint64 = EXCLUDED.Uint64, Int64 = EXCLUDED.Int64, Float = EXCLUDED.Float, Labels = EXCLUDED.Labels, Timestamp = EXCLUDED.Timestamp, Enum = EXCLUDED.Enum, Enums = EXCLUDED.Enums, serialized = EXCLUDED.serialized"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	return nil
}

func (s *storeImpl) copyFromSinglekey(ctx context.Context, tx pgx.Tx, objs ...*storage.TestSingleKeyStruct) error {

	inputRows := [][]interface{}{}

	var err error

	// This is a copy so first we must delete the rows and re-add them
	// Which is essentially the desired behaviour of an upsert.
	var deletes []string

	copyCols := []string{

		"key",

		"name",

		"stringslice",

		"bool",

		"uint64",

		"int64",

		"float",

		"labels",

		"timestamp",

		"enum",

		"enums",

		"serialized",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		serialized, marshalErr := obj.Marshal()
		if marshalErr != nil {
			return marshalErr
		}

		inputRows = append(inputRows, []interface{}{

			obj.GetKey(),

			obj.GetName(),

			obj.GetStringSlice(),

			obj.GetBool(),

			obj.GetUint64(),

			obj.GetInt64(),

			obj.GetFloat(),

			obj.GetLabels(),

			pgutils.NilOrTime(obj.GetTimestamp()),

			obj.GetEnum(),

			obj.GetEnums(),

			serialized,
		})

		// Add the id to be deleted.
		deletes = append(deletes, obj.GetKey())

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			if err := s.DeleteMany(ctx, deletes); err != nil {
				return err
			}
			// clear the inserts and vals for the next batch
			deletes = nil

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"singlekey"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	return err
}

func (s *storeImpl) copyFrom(ctx context.Context, objs ...*storage.TestSingleKeyStruct) error {
	conn, release, err := s.acquireConn(ctx, ops.Get, "TestSingleKeyStruct")
	if err != nil {
		return err
	}
	defer release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}

	if err := s.copyFromSinglekey(ctx, tx, objs...); err != nil {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *storeImpl) upsert(ctx context.Context, objs ...*storage.TestSingleKeyStruct) error {
	conn, release, err := s.acquireConn(ctx, ops.Get, "TestSingleKeyStruct")
	if err != nil {
		return err
	}
	defer release()

	for _, obj := range objs {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}

		if err := insertIntoSinglekey(ctx, tx, obj); err != nil {
			if err := tx.Rollback(ctx); err != nil {
				return err
			}
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *storeImpl) Upsert(ctx context.Context, obj *storage.TestSingleKeyStruct) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Upsert, "TestSingleKeyStruct")

	return s.upsert(ctx, obj)
}

func (s *storeImpl) UpsertMany(ctx context.Context, objs []*storage.TestSingleKeyStruct) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.UpdateMany, "TestSingleKeyStruct")

	if len(objs) < batchAfter {
		return s.upsert(ctx, objs...)
	} else {
		return s.copyFrom(ctx, objs...)
	}
}

// Count returns the number of objects in the store
func (s *storeImpl) Count(ctx context.Context) (int, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Count, "TestSingleKeyStruct")

	var sacQueryFilter *v1.Query

	return postgres.RunCountRequestForSchema(schema, sacQueryFilter, s.db)
}

// Exists returns if the id exists in the store
func (s *storeImpl) Exists(ctx context.Context, key string) (bool, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Exists, "TestSingleKeyStruct")

	var sacQueryFilter *v1.Query

	q := search.ConjunctionQuery(
		sacQueryFilter,
		search.NewQueryBuilder().AddDocIDs(key).ProtoQuery(),
	)

	count, err := postgres.RunCountRequestForSchema(schema, q, s.db)
	return count == 1, err
}

// Get returns the object, if it exists from the store
func (s *storeImpl) Get(ctx context.Context, key string) (*storage.TestSingleKeyStruct, bool, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Get, "TestSingleKeyStruct")

	conn, release, err := s.acquireConn(ctx, ops.Get, "TestSingleKeyStruct")
	if err != nil {
		return nil, false, err
	}
	defer release()

	row := conn.QueryRow(ctx, getStmt, key)
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, false, pgutils.ErrNilIfNoRows(err)
	}

	var msg storage.TestSingleKeyStruct
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, false, err
	}
	return &msg, true, nil
}
func (s *storeImpl) GetAll(ctx context.Context) ([]*storage.TestSingleKeyStruct, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.GetAll, "TestSingleKeyStruct")

	var objs []*storage.TestSingleKeyStruct
	err := s.Walk(ctx, func(obj *storage.TestSingleKeyStruct) error {
		objs = append(objs, obj)
		return nil
	})
	return objs, err
}

func (s *storeImpl) acquireConn(ctx context.Context, op ops.Op, typ string) (*pgxpool.Conn, func(), error) {
	defer metrics.SetAcquireDBConnDuration(time.Now(), op, typ)
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	return conn, conn.Release, nil
}

// Delete removes the specified ID from the store
func (s *storeImpl) Delete(ctx context.Context, key string) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Remove, "TestSingleKeyStruct")

	var sacQueryFilter *v1.Query

	q := search.ConjunctionQuery(
		sacQueryFilter,
		search.NewQueryBuilder().AddDocIDs(key).ProtoQuery(),
	)

	return postgres.RunDeleteRequestForSchema(schema, q, s.db)
}

// GetIDs returns all the IDs for the store
func (s *storeImpl) GetIDs(ctx context.Context) ([]string, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.GetAll, "storage.TestSingleKeyStructIDs")
	var sacQueryFilter *v1.Query

	result, err := postgres.RunSearchRequestForSchema(schema, sacQueryFilter, s.db)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(result))
	for _, entry := range result {
		ids = append(ids, entry.ID)
	}

	return ids, nil
}

// GetMany returns the objects specified by the IDs or the index in the missing indices slice
func (s *storeImpl) GetMany(ctx context.Context, ids []string) ([]*storage.TestSingleKeyStruct, []int, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.GetMany, "TestSingleKeyStruct")

	conn, release, err := s.acquireConn(ctx, ops.GetMany, "TestSingleKeyStruct")
	if err != nil {
		return nil, nil, err
	}
	defer release()

	rows, err := conn.Query(ctx, getManyStmt, ids)
	if err != nil {
		if err == pgx.ErrNoRows {
			missingIndices := make([]int, 0, len(ids))
			for i := range ids {
				missingIndices = append(missingIndices, i)
			}
			return nil, missingIndices, nil
		}
		return nil, nil, err
	}
	defer rows.Close()
	resultsByID := make(map[string]*storage.TestSingleKeyStruct)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, nil, err
		}
		msg := &storage.TestSingleKeyStruct{}
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, nil, err
		}
		resultsByID[msg.GetKey()] = msg
	}
	missingIndices := make([]int, 0, len(ids)-len(resultsByID))
	// It is important that the elems are populated in the same order as the input ids
	// slice, since some calling code relies on that to maintain order.
	elems := make([]*storage.TestSingleKeyStruct, 0, len(resultsByID))
	for i, id := range ids {
		if result, ok := resultsByID[id]; !ok {
			missingIndices = append(missingIndices, i)
		} else {
			elems = append(elems, result)
		}
	}
	return elems, missingIndices, nil
}

// Delete removes the specified IDs from the store
func (s *storeImpl) DeleteMany(ctx context.Context, ids []string) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.RemoveMany, "TestSingleKeyStruct")

	var sacQueryFilter *v1.Query

	q := search.ConjunctionQuery(
		sacQueryFilter,
		search.NewQueryBuilder().AddDocIDs(ids...).ProtoQuery(),
	)

	return postgres.RunDeleteRequestForSchema(schema, q, s.db)
}

// Walk iterates over all of the objects in the store and applies the closure
func (s *storeImpl) Walk(ctx context.Context, fn func(obj *storage.TestSingleKeyStruct) error) error {
	rows, err := s.db.Query(ctx, walkStmt)
	if err != nil {
		return pgutils.ErrNilIfNoRows(err)
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var msg storage.TestSingleKeyStruct
		if err := proto.Unmarshal(data, &msg); err != nil {
			return err
		}
		if err := fn(&msg); err != nil {
			return err
		}
	}
	return nil
}

//// Used for testing

func dropTableSinglekey(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS singlekey CASCADE")

}

func Destroy(ctx context.Context, db *pgxpool.Pool) {
	dropTableSinglekey(ctx, db)
}

//// Stubs for satisfying legacy interfaces

// AckKeysIndexed acknowledges the passed keys were indexed
func (s *storeImpl) AckKeysIndexed(ctx context.Context, keys ...string) error {
	return nil
}

// GetKeysToIndex returns the keys that need to be indexed
func (s *storeImpl) GetKeysToIndex(ctx context.Context) ([]string, error) {
	return nil, nil
}
