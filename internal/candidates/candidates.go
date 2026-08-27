package candidates

import (
	"bufio"
	"os"
	"sort"
	"strings"
)

var Builtin = []string{
	"admin", "debug", "include", "include_deleted", "internal", "limit", "offset", "page", "page_size", "preview", "role", "sort", "order", "user_id", "account_id", "organization_id", "workspace_id", "environment_id", "format", "fields", "expand", "cursor", "filter", "status", "verbose", "test",
}

func Load(path string) ([]string, error) {
	set := map[string]struct{}{}
	for _, s := range Builtin {
		set[s] = struct{}{}
	}
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			s := strings.TrimSpace(sc.Text())
			if s != "" && !strings.HasPrefix(s, "#") {
				set[s] = struct{}{}
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}
