//go:build !linux

package helper

import (
	"errors"
	"os"
)

// OpenTun 仅 Linux 支持(SCM_RIGHTS 收 fd)。
func (c *Client) OpenTun(name string) (*os.File, error) {
	return nil, errors.New("TUN 助手仅支持 Linux")
}
