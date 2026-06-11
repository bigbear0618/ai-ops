package edge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	model "github.com/ongridio/ongrid/internal/manager/model/edge"
	"github.com/ongridio/ongrid/internal/pkg/tunnel"
)

type fakePluginConfigRepo struct {
	rows map[string]*model.PluginConfig
}

func newFakePluginConfigRepo() *fakePluginConfigRepo {
	return &fakePluginConfigRepo{rows: map[string]*model.PluginConfig{}}
}

func (r *fakePluginConfigRepo) ListByEdge(_ context.Context, edgeID uint64) ([]*model.PluginConfig, error) {
	out := []*model.PluginConfig{}
	for _, row := range r.rows {
		if row.EdgeID == edgeID {
			cp := *row
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakePluginConfigRepo) Get(_ context.Context, edgeID uint64, plugin string) (*model.PluginConfig, error) {
	row := r.rows[plugin]
	if row == nil || row.EdgeID != edgeID {
		return nil, nil
	}
	cp := *row
	return &cp, nil
}

func (r *fakePluginConfigRepo) Upsert(_ context.Context, in *model.PluginConfig) (*model.PluginConfig, error) {
	cp := *in
	cp.ID = 1
	r.rows[in.PluginName] = &cp
	return &cp, nil
}

func (r *fakePluginConfigRepo) Delete(_ context.Context, _ uint64, plugin string) error {
	delete(r.rows, plugin)
	return nil
}

func (r *fakePluginConfigRepo) CountByPlugin(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

type fakeEndpointResolver struct{}

func (fakeEndpointResolver) Endpoint(_ context.Context, plugin string) string {
	return "http://manager/" + plugin
}

type fakeDatabaseSecretWriter struct {
	reqs []tunnel.WriteDatabaseMetricsSecretRequest
}

func (w *fakeDatabaseSecretWriter) WriteDatabaseMetricsSecret(_ context.Context, _ uint64, req tunnel.WriteDatabaseMetricsSecretRequest) error {
	w.reqs = append(w.reqs, req)
	return nil
}

func TestSetDatabaseMetricsWritesSecretAndStripsCredentials(t *testing.T) {
	repo := newFakePluginConfigRepo()
	writer := &fakeDatabaseSecretWriter{}
	uc := NewPluginConfigUC(repo, nil, fakeEndpointResolver{}, nil)
	uc.SetDatabaseMetricsSecretWriter(writer)

	row, err := uc.Set(context.Background(), 7, model.PluginNameDatabaseMetrics, SetInput{
		Enabled: true,
		Spec: map[string]interface{}{
			"sources": []interface{}{
				map[string]interface{}{
					"id":             "pg-prod",
					"db_type":        "postgresql",
					"name":           "pg-prod",
					"listen_address": "127.0.0.1:19187",
					"credentials": map[string]interface{}{
						"host":     "127.0.0.1",
						"port":     "15432",
						"username": "u",
						"password": "p-secret",
						"database": "postgres",
						"sslmode":  "disable",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if len(writer.reqs) != 1 {
		t.Fatalf("secret writes = %d, want 1", len(writer.reqs))
	}
	req := writer.reqs[0]
	if req.SourceID != "pg-prod" {
		t.Fatalf("SourceID = %q, want pg-prod", req.SourceID)
	}
	if req.Path != "/var/lib/ongrid-edge/secrets/pg-prod.dsn" {
		t.Fatalf("Path = %q", req.Path)
	}
	if !strings.Contains(req.Content, "postgresql://u:p-secret@127.0.0.1:15432/postgres?sslmode=disable") {
		t.Fatalf("Content = %q", req.Content)
	}
	blob, err := json.Marshal(row.Spec)
	if err != nil {
		t.Fatalf("Marshal(spec) error = %v", err)
	}
	if strings.Contains(string(blob), "p-secret") || strings.Contains(string(blob), "credentials") {
		t.Fatalf("stored spec leaked credentials: %s", blob)
	}
	connection := row.Spec["sources"].([]interface{})[0].(map[string]interface{})["connection"].(map[string]interface{})
	if connection["type"] != "managed" || connection["secret_set"] != true {
		t.Fatalf("connection = %#v", connection)
	}
}
