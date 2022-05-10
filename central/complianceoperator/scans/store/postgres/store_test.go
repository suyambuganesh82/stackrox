// Code generated by pg-bindings generator. DO NOT EDIT.

//go:build sql_integration

package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/features"
	"github.com/stackrox/rox/pkg/postgres/pgtest"
	"github.com/stackrox/rox/pkg/sac"
	"github.com/stackrox/rox/pkg/testutils"
	"github.com/stackrox/rox/pkg/testutils/envisolator"
	"github.com/stretchr/testify/suite"
)

type ComplianceoperatorscansStoreSuite struct {
	suite.Suite
	envIsolator *envisolator.EnvIsolator
	store       Store
	pool        *pgxpool.Pool
}

func TestComplianceoperatorscansStore(t *testing.T) {
	suite.Run(t, new(ComplianceoperatorscansStoreSuite))
}

func (s *ComplianceoperatorscansStoreSuite) SetupTest() {
	s.envIsolator = envisolator.NewEnvIsolator(s.T())
	s.envIsolator.Setenv(features.PostgresDatastore.EnvVar(), "true")

	if !features.PostgresDatastore.Enabled() {
		s.T().Skip("Skip postgres store tests")
		s.T().SkipNow()
	}

	ctx := sac.WithAllAccess(context.Background())

	source := pgtest.GetConnectionString(s.T())
	config, err := pgxpool.ParseConfig(source)
	s.Require().NoError(err)
	pool, err := pgxpool.ConnectConfig(ctx, config)
	s.Require().NoError(err)

	Destroy(ctx, pool)

	s.pool = pool
	s.store = New(ctx, pool)
}

func (s *ComplianceoperatorscansStoreSuite) TearDownTest() {
	if s.pool != nil {
		s.pool.Close()
	}
	s.envIsolator.RestoreAll()
}

func (s *ComplianceoperatorscansStoreSuite) TestStore() {
	ctx := sac.WithAllAccess(context.Background())

	store := s.store

	complianceOperatorScan := &storage.ComplianceOperatorScan{}
	s.NoError(testutils.FullInit(complianceOperatorScan, testutils.SimpleInitializer(), testutils.JSONFieldsFilter))

	foundComplianceOperatorScan, exists, err := store.Get(ctx, complianceOperatorScan.GetId())
	s.NoError(err)
	s.False(exists)
	s.Nil(foundComplianceOperatorScan)

	withNoAccessCtx := sac.WithNoAccess(ctx)

	s.NoError(store.Upsert(ctx, complianceOperatorScan))
	foundComplianceOperatorScan, exists, err = store.Get(ctx, complianceOperatorScan.GetId())
	s.NoError(err)
	s.True(exists)
	s.Equal(complianceOperatorScan, foundComplianceOperatorScan)

	complianceOperatorScanCount, err := store.Count(ctx)
	s.NoError(err)
	s.Equal(1, complianceOperatorScanCount)
	complianceOperatorScanCount, err = store.Count(withNoAccessCtx)
	s.NoError(err)
	s.Zero(complianceOperatorScanCount)

	complianceOperatorScanExists, err := store.Exists(ctx, complianceOperatorScan.GetId())
	s.NoError(err)
	s.True(complianceOperatorScanExists)
	s.NoError(store.Upsert(ctx, complianceOperatorScan))
	s.ErrorIs(store.Upsert(withNoAccessCtx, complianceOperatorScan), sac.ErrResourceAccessDenied)

	foundComplianceOperatorScan, exists, err = store.Get(ctx, complianceOperatorScan.GetId())
	s.NoError(err)
	s.True(exists)
	s.Equal(complianceOperatorScan, foundComplianceOperatorScan)

	s.NoError(store.Delete(ctx, complianceOperatorScan.GetId()))
	foundComplianceOperatorScan, exists, err = store.Get(ctx, complianceOperatorScan.GetId())
	s.NoError(err)
	s.False(exists)
	s.Nil(foundComplianceOperatorScan)
	s.ErrorIs(store.Delete(withNoAccessCtx, complianceOperatorScan.GetId()), sac.ErrResourceAccessDenied)

	var complianceOperatorScans []*storage.ComplianceOperatorScan
	for i := 0; i < 200; i++ {
		complianceOperatorScan := &storage.ComplianceOperatorScan{}
		s.NoError(testutils.FullInit(complianceOperatorScan, testutils.UniqueInitializer(), testutils.JSONFieldsFilter))
		complianceOperatorScans = append(complianceOperatorScans, complianceOperatorScan)
	}

	s.NoError(store.UpsertMany(ctx, complianceOperatorScans))

	complianceOperatorScanCount, err = store.Count(ctx)
	s.NoError(err)
	s.Equal(200, complianceOperatorScanCount)
}
