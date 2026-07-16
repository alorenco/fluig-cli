//go:build !windows

package devserver

import (
	"os/exec"
	"syscall"
)

// setProcGroup põe o npm num grupo de processos próprio — o watch do Vite
// cria filhos (node/esbuild) que precisam morrer junto.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killTree encerra o grupo inteiro (npm + node + esbuild).
func killTree(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}
