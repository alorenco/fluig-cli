//go:build integration

package fluig

import (
	"context"
	"errors"
	"testing"
)

// Consulta de solicitações (read-only) contra a homologação.
func TestIntegrationRequests(t *testing.T) {
	c, err := NewClient(integrationOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	reqs, err := c.ListRequests(ctx, RequestFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if len(reqs) == 0 {
		t.Skip("homologação sem solicitações — nada a validar")
	}
	t.Logf("%d solicitações; primeira: #%d %s status=%s etapa=%v",
		len(reqs), reqs[0].ID, reqs[0].ProcessID, reqs[0].Status, reqs[0].CurrentSteps)
	if reqs[0].ID == 0 || reqs[0].ProcessID == "" || reqs[0].Status == "" {
		t.Errorf("campos básicos vazios: %+v", reqs[0])
	}
	if reqs[0].Requester == nil {
		t.Error("requester deveria vir expandido")
	}

	// show + tasks da primeira solicitação.
	r, err := c.GetRequest(ctx, reqs[0].ID)
	if err != nil {
		t.Fatalf("GetRequest(%d): %v", reqs[0].ID, err)
	}
	if r.ID != reqs[0].ID {
		t.Errorf("GetRequest devolveu #%d, quer #%d", r.ID, reqs[0].ID)
	}
	tasks, err := c.RequestTasks(ctx, reqs[0].ID)
	if err != nil {
		t.Fatalf("RequestTasks: %v", err)
	}
	if len(tasks) == 0 {
		t.Error("toda solicitação tem ao menos uma tarefa no histórico")
	}
	t.Logf("%d tarefas; última: mov=%d etapa=%q status=%s",
		len(tasks), tasks[len(tasks)-1].Movement, tasks[len(tasks)-1].StateName, tasks[len(tasks)-1].Status)

	// Inexistente: 404 real → ErrNotFound.
	if _, err := c.GetRequest(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRequest(1) deveria dar ErrNotFound, veio %v", err)
	}
}
