package discover

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

func DetectProjectOwner(projectDir string) (string, error) {
	info, err := os.Stat(projectDir)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", projectDir, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("could not get file owner for %s", projectDir)
	}

	u, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		return fmt.Sprintf("uid:%d", stat.Uid), nil
	}

	return u.Username, nil
}
