//go:build windows

package devserver

import "os/exec"

// setProcGroup: no Windows o Kill já encerra o processo; os filhos do npm
// morrem quando o console fecha (sem equivalente simples de process group).
func setProcGroup(_ *exec.Cmd) {}

func killTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
