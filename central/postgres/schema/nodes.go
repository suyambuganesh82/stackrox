// Code generated by pg-bindings generator. DO NOT EDIT.

package schema

import (
	"fmt"
	"reflect"

	"github.com/stackrox/rox/central/globaldb"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/postgres"
	"github.com/stackrox/rox/pkg/postgres/walker"
	"github.com/stackrox/rox/pkg/search"
)

var (
	// CreateTableNodesStmt holds the create statement for table `nodes`.
	CreateTableNodesStmt = &postgres.CreateStmts{
		Table: `
               create table if not exists nodes (
                   Id varchar,
                   Name varchar,
                   ClusterId varchar,
                   ClusterName varchar,
                   Labels jsonb,
                   Annotations jsonb,
                   JoinedAt timestamp,
                   ContainerRuntime_Version varchar,
                   OsImage varchar,
                   LastUpdated timestamp,
                   Scan_ScanTime timestamp,
                   Components integer,
                   Cves integer,
                   FixableCves integer,
                   RiskScore numeric,
                   TopCvss numeric,
                   serialized bytea,
                   PRIMARY KEY(Id)
               )
               `,
		Indexes: []string{},
		Children: []*postgres.CreateStmts{
			&postgres.CreateStmts{
				Table: `
               create table if not exists nodes_Taints (
                   nodes_Id varchar,
                   idx integer,
                   Key varchar,
                   Value varchar,
                   TaintEffect integer,
                   PRIMARY KEY(nodes_Id, idx),
                   CONSTRAINT fk_parent_table_0 FOREIGN KEY (nodes_Id) REFERENCES nodes(Id) ON DELETE CASCADE
               )
               `,
				Indexes: []string{
					"create index if not exists nodesTaints_idx on nodes_Taints using btree(idx)",
				},
				Children: []*postgres.CreateStmts{},
			},
			&postgres.CreateStmts{
				Table: `
               create table if not exists nodes_Components (
                   nodes_Id varchar,
                   idx integer,
                   Name varchar,
                   Version varchar,
                   PRIMARY KEY(nodes_Id, idx),
                   CONSTRAINT fk_parent_table_0 FOREIGN KEY (nodes_Id) REFERENCES nodes(Id) ON DELETE CASCADE
               )
               `,
				Indexes: []string{
					"create index if not exists nodesComponents_idx on nodes_Components using btree(idx)",
				},
				Children: []*postgres.CreateStmts{
					&postgres.CreateStmts{
						Table: `
               create table if not exists nodes_Components_Vulns (
                   nodes_Id varchar,
                   nodes_Components_idx integer,
                   idx integer,
                   Cve varchar,
                   Cvss numeric,
                   FixedBy varchar,
                   PublishedOn timestamp,
                   Suppressed bool,
                   State integer,
                   PRIMARY KEY(nodes_Id, nodes_Components_idx, idx),
                   CONSTRAINT fk_parent_table_0 FOREIGN KEY (nodes_Id, nodes_Components_idx) REFERENCES nodes_Components(nodes_Id, idx) ON DELETE CASCADE
               )
               `,
						Indexes: []string{
							"create index if not exists nodesComponentsVulns_idx on nodes_Components_Vulns using btree(idx)",
						},
						Children: []*postgres.CreateStmts{},
					},
				},
			},
		},
	}

	// NodesSchema is the go schema for table `nodes`.
	NodesSchema = func() *walker.Schema {
		schema := globaldb.GetSchemaForTable("nodes")
		if schema != nil {
			return schema
		}
		schema = walker.Walk(reflect.TypeOf((*storage.Node)(nil)), "nodes")
		referencedSchemas := map[string]*walker.Schema{
			"storage.Cluster": ClustersSchema,
		}

		schema.ResolveReferences(func(messageTypeName string) *walker.Schema {
			return referencedSchemas[fmt.Sprintf("storage.%s", messageTypeName)]
		})
		schema.SetOptionsMap(search.Walk(v1.SearchCategory_NODES, "node", (*storage.Node)(nil)))
		globaldb.RegisterTable(schema)
		return schema
	}()
)
