package update

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// checkState é o resultado persistido da checagem periódica de versão.
type checkState struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
}

// NoticeCachePath devolve o arquivo de cache da checagem de versão
// (<cache do usuário>/fluigcli/update-check.json).
func NoticeCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fluigcli", "update-check.json"), nil
}

// CheckForNotice devolve a última versão conhecida e se ela é mais nova que a
// atual, consultando o GitHub no máximo uma vez por maxAge (resultado fica em
// cache em cachePath, inclusive quando a consulta falha, para não insistir
// offline). É melhor-esforço: qualquer falha resulta em ("", false).
func CheckForNotice(ctx context.Context, c *Client, cachePath, current string, now time.Time, maxAge time.Duration) (string, bool) {
	var st checkState
	if raw, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(raw, &st)
	}

	if st.CheckedAt.IsZero() || now.Sub(st.CheckedAt) > maxAge {
		st = checkState{CheckedAt: now}
		if latest, err := c.Latest(ctx); err == nil {
			st.Latest = latest
		}
		if raw, err := json.Marshal(st); err == nil {
			_ = os.MkdirAll(filepath.Dir(cachePath), 0o700)
			_ = os.WriteFile(cachePath, raw, 0o600)
		}
	}
	return st.Latest, IsNewer(st.Latest, current)
}
