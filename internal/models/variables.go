package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

var variableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Variables map[string]string

type VariableEntry struct {
	Key   string
	Value string
}

func (v Variables) Value() (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	data, err := json.Marshal(map[string]string(v))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (v *Variables) Scan(src any) error {
	if src == nil {
		*v = Variables{}
		return nil
	}

	var data []byte
	switch s := src.(type) {
	case []byte:
		data = s
	case string:
		data = []byte(s)
	default:
		return fmt.Errorf("unsupported variables type %T", src)
	}

	if len(data) == 0 {
		*v = Variables{}
		return nil
	}

	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*v = Variables(values)
	return nil
}

func ParseVariables(keys []string, values []string) (Variables, error) {
	vars := Variables{}
	seen := map[string]struct{}{}

	for i, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if !variableNamePattern.MatchString(key) {
			return nil, fmt.Errorf("变量名 %q 不合法，只能使用字母、数字和下划线，且不能以数字开头", key)
		}
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("变量名 %q 重复", key)
		}
		seen[key] = struct{}{}

		value := ""
		if i < len(values) {
			value = values[i]
		}
		vars[key] = value
	}

	return vars, nil
}

func ParseVariablesFromRequest(r *http.Request) (Variables, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	return ParseVariables(r.Form["variable_key"], r.Form["variable_value"])
}

func MergeVariables(variableSets ...Variables) Variables {
	merged := Variables{}
	for _, vars := range variableSets {
		for key, value := range vars {
			merged[key] = value
		}
	}
	return merged
}

func (v Variables) Env() []string {
	env := make([]string, 0, len(v))
	for key, value := range v {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	return env
}

func (v Variables) Entries() []VariableEntry {
	entries := make([]VariableEntry, 0, len(v))
	for key, value := range v {
		entries = append(entries, VariableEntry{Key: key, Value: value})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries
}
