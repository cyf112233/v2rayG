package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// proxyPrev 保存修改系统代理前的状态,用于恢复。
type proxyPrev struct {
	Mode        string            `json:"mode"`
	Hosts       map[string]string `json:"hosts"`
	IgnoreHosts []string          `json:"ignoreHosts"`
}

const gsettingsSchema = "org.gnome.system.proxy"

func proxyPrevPath() string {
	return filepath.Join(ConfigDir(), "proxy-prev.json")
}

// SetSystemProxy 通过 gsettings 将 GNOME 系统代理设为手动并指向本机端口。
// 修改前保存原状态到 proxy-prev.json;任一步骤失败即返回错误。
func SetSystemProxy(st Settings) error {
	if st.SocksPort == 0 {
		st.SocksPort = 10808
	}
	if st.HTTPPort == 0 {
		st.HTTPPort = 10809
	}
	prev := proxyPrev{
		Mode:  gsettingsGet("mode"),
		Hosts: map[string]string{},
	}
	for _, k := range []string{"http host", "http port", "https host", "https port", "socks host", "socks port"} {
		prev.Hosts[k] = gsettingsGet(k)
	}
	prev.IgnoreHosts = gsettingsGetArray("ignore-hosts")
	if err := saveProxyPrev(prev); err != nil {
		return err
	}
	steps := [][2]string{
		{"mode", "manual"},
		{"http host", "127.0.0.1"},
		{"http port", strconv.Itoa(st.HTTPPort)},
		{"https host", "127.0.0.1"},
		{"https port", strconv.Itoa(st.HTTPPort)},
		{"socks host", "127.0.0.1"},
		{"socks port", strconv.Itoa(st.SocksPort)},
		{"ignore-hosts", gsettingsArrayStr(ignoreList(st.ProxyIgnore))},
	}
	for _, step := range steps {
		if err := gsettingsSet(step[0], step[1]); err != nil {
			return err
		}
	}
	return nil
}

// RestoreSystemProxy 恢复系统代理:有保存记录则还原,否则设为 none;尽力而为不报错。
func RestoreSystemProxy() error {
	prev, err := loadProxyPrev()
	if err != nil {
		_ = gsettingsSet("mode", "none")
		return nil
	}
	_ = gsettingsSet("mode", prev.Mode)
	for k, v := range prev.Hosts {
		_ = gsettingsSet(k, v)
	}
	if len(prev.IgnoreHosts) > 0 {
		_ = gsettingsSetArray("ignore-hosts", prev.IgnoreHosts)
	} else {
		_ = gsettingsSet("ignore-hosts", "[]")
	}
	_ = os.Remove(proxyPrevPath())
	return nil
}

func saveProxyPrev(prev proxyPrev) error {
	data, err := json.Marshal(prev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return err
	}
	return os.WriteFile(proxyPrevPath(), data, 0644)
}

func loadProxyPrev() (*proxyPrev, error) {
	data, err := os.ReadFile(proxyPrevPath())
	if err != nil {
		return nil, err
	}
	var prev proxyPrev
	if err := json.Unmarshal(data, &prev); err != nil {
		return nil, err
	}
	return &prev, nil
}

func gsettingsGet(key string) string {
	out, err := exec.Command("gsettings", "get", gsettingsSchema, key).Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	return strings.Trim(s, "'")
}

func gsettingsGetArray(key string) []string {
	out, err := exec.Command("gsettings", "get", gsettingsSchema, key).Output()
	if err != nil {
		return nil
	}
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	arr := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "'")
		if p != "" {
			arr = append(arr, p)
		}
	}
	return arr
}

func gsettingsSet(key, value string) error {
	return exec.Command("gsettings", "set", gsettingsSchema, key, value).Run()
}

func gsettingsSetArray(key string, arr []string) error {
	quoted := make([]string, 0, len(arr))
	for _, a := range arr {
		quoted = append(quoted, "'"+a+"'")
	}
	return gsettingsSet(key, "["+strings.Join(quoted, ",")+"]")
}

// ignoreList 拆分系统代理忽略列表;空值时用默认值。
func ignoreList(s string) []string {
	if strings.TrimSpace(s) == "" {
		s = "localhost,127.0.0.0/8,::1"
	}
	return splitComma(s)
}

// gsettingsArrayStr 把字符串列表格式化为 gsettings 的数组字面量,如 ['a','b']。
func gsettingsArrayStr(arr []string) string {
	quoted := make([]string, 0, len(arr))
	for _, a := range arr {
		quoted = append(quoted, "'"+a+"'")
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
