package fluig

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ServerMonitor é um monitor de serviço do servidor (environment/v2).
type ServerMonitor struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"` // OK | FAILURE | NONE
	SuccessRate float64 `json:"successRate"`
}

// ServerMonitors lista os monitores de serviços do servidor. Requer usuário
// com privilégio administrativo (sem ele o módulo responde 401).
func (c *Client) ServerMonitors(ctx context.Context) ([]ServerMonitor, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	endpoint := c.url("/environment/api/v2/monitors") + "?expand=sucessRate"
	body, status, err := c.doJSON(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("environment/v2/monitors", status, body)
	}
	var parsed struct {
		Items []struct {
			Name        string      `json:"name"`
			Status      string      `json:"status"`
			SuccessRate json.Number `json:"sucessRate"` // grafia da API
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta inesperada de environment/v2/monitors: %w", err)
	}
	out := make([]ServerMonitor, 0, len(parsed.Items))
	for _, it := range parsed.Items {
		rate, _ := it.SuccessRate.Float64()
		out = append(out, ServerMonitor{Name: it.Name, Status: it.Status, SuccessRate: rate})
	}
	return out, nil
}

// ServerStats resume as estatísticas do servidor (environment/v2/statistics).
// Campos tipados para o contrato --json ficar estável entre versões do Fluig.
type ServerStats struct {
	ConnectedUsers  int    `json:"connectedUsers"`
	UptimeMillis    int64  `json:"uptimeMillis"`
	HeapUsed        int64  `json:"heapUsedBytes"`
	NonHeapUsed     int64  `json:"nonHeapUsedBytes"`
	ThreadCount     int    `json:"threadCount"`
	ThreadPeak      int    `json:"threadPeak"`
	DatabaseName    string `json:"databaseName"`
	DatabaseVersion string `json:"databaseVersion"`
	DatabaseSize    int64  `json:"databaseSizeBytes"`
	OSMemoryTotal   int64  `json:"osMemoryTotalBytes"`
	OSMemoryFree    int64  `json:"osMemoryFreeBytes"`
}

// ServerStatistics consulta as estatísticas do servidor (requer admin).
func (c *Client) ServerStatistics(ctx context.Context) (*ServerStats, error) {
	if err := c.EnsureSession(ctx); err != nil {
		return nil, err
	}
	body, status, err := c.doJSON(ctx, http.MethodGet, c.url("/environment/api/v2/statistics"), nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, restRequestError("environment/v2/statistics", status, body)
	}
	var raw struct {
		ConnectedUsers struct {
			ConnectedUsers int `json:"connectedUsers"`
		} `json:"CONNECTED_USERS"`
		Runtime struct {
			Uptime int64 `json:"uptime"`
		} `json:"RUNTIME"`
		Memory struct {
			Heap    int64 `json:"heap-memory-usage"`
			NonHeap int64 `json:"non-heap-memory-usage"`
		} `json:"MEMORY"`
		Threading struct {
			Count     int `json:"count"`
			PeakCount int `json:"peakCount"`
		} `json:"THREADING"`
		DatabaseInfo struct {
			Name    string `json:"databaseName"`
			Version string `json:"databaseVersion"`
		} `json:"DATABASE_INFO"`
		DatabaseSize struct {
			Size int64 `json:"size"`
		} `json:"DATABASE_SIZE"`
		OS struct {
			MemTotal int64 `json:"server-memory-size"`
			MemFree  int64 `json:"server-memory-free"`
		} `json:"OPERATION_SYSTEM"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("resposta inesperada de environment/v2/statistics: %w", err)
	}
	return &ServerStats{
		ConnectedUsers:  raw.ConnectedUsers.ConnectedUsers,
		UptimeMillis:    raw.Runtime.Uptime,
		HeapUsed:        raw.Memory.Heap,
		NonHeapUsed:     raw.Memory.NonHeap,
		ThreadCount:     raw.Threading.Count,
		ThreadPeak:      raw.Threading.PeakCount,
		DatabaseName:    raw.DatabaseInfo.Name,
		DatabaseVersion: raw.DatabaseInfo.Version,
		DatabaseSize:    raw.DatabaseSize.Size,
		OSMemoryTotal:   raw.OS.MemTotal,
		OSMemoryFree:    raw.OS.MemFree,
	}, nil
}
