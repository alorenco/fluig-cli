package update

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// CurrentExecutable resolve o caminho real do binário em execução (seguindo
// symlinks, para atualizar o alvo e não o link).
func CurrentExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

// ReplaceExecutable instala newBin no lugar de target. A troca é feita por
// rename no mesmo diretório, então é atômica para quem estiver executando.
func ReplaceExecutable(newBin, target string) error {
	return replaceExecutable(newBin, target, runtime.GOOS)
}

func replaceExecutable(newBin, target, goos string) error {
	// Copia para o diretório do alvo antes de renomear: rename entre
	// filesystems (ex.: /tmp → /usr/local/bin) falharia.
	staging := target + ".new"
	if err := copyFile(newBin, staging); err != nil {
		return err
	}

	if goos == "windows" {
		// O Windows não deixa sobrescrever um executável em uso, mas deixa
		// renomeá-lo: o atual vira .old e o novo assume o lugar.
		old := target + ".old"
		_ = os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			_ = os.Remove(staging)
			return err
		}
		if err := os.Rename(staging, target); err != nil {
			_ = os.Rename(old, target)
			_ = os.Remove(staging)
			return err
		}
		return nil
	}

	if err := os.Rename(staging, target); err != nil {
		_ = os.Remove(staging)
		return err
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dest)
		return err
	}
	return out.Close()
}
