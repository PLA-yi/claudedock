package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// migration0014Filename 是 Phase 30 Plan 01 落地的 migration 文件名。
// 对齐 30-CONTEXT D-10（紧随现存 0013 之后，无跳号）。
const migration0014Filename = "0014_claude_account_persistent_volume.sql"

// TestMigration0014_FileContent 验证 migration 0014 的内容语义（D-01/D-02/D-10）。
//   - 使用 ADD COLUMN IF NOT EXISTS 保持幂等，兼容空库与 v2.0 升级库。
//   - 列类型 TEXT，且不能使用空字符串默认值（避免三态，NULL = 未分配）。
//   - 文件中必须包含 down 路径注释（DROP COLUMN IF EXISTS）以便后续回滚指令清晰。
func TestMigration0014_FileContent(t *testing.T) {
	path := filepath.Join("..", "migrations", migration0014Filename)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 migration 失败（%s）: %v", migration0014Filename, err)
	}
	content := string(raw)

	mustContain := []string{
		"claude_accounts",
		"persistent_volume_name",
		"ADD COLUMN IF NOT EXISTS persistent_volume_name",
		"TEXT",
		"DROP COLUMN IF EXISTS persistent_volume_name",
	}
	for _, token := range mustContain {
		if !strings.Contains(content, token) {
			t.Errorf("migration 0014 必须包含 %q，当前内容:\n%s", token, content)
		}
	}

	forbidden := []string{
		"DEFAULT ''",
		"DEFAULT \"\"",
		"NOT NULL",
	}
	for _, token := range forbidden {
		if strings.Contains(content, token) {
			t.Errorf("migration 0014 不得包含 %q（D-02：NULL 表示未分配，禁止三态）", token)
		}
	}
}

// TestClaudeAccount_PersistentVolumeNameNullable 验证仓储模型通过 *string 支持 NULL 语义。
// D-02：NULL = 未分配；非空字符串 = 已分配的 Docker named volume 名称。
func TestClaudeAccount_PersistentVolumeNameNullable(t *testing.T) {
	typ := reflect.TypeOf(ClaudeAccount{})
	field, ok := typ.FieldByName("PersistentVolumeName")
	if !ok {
		t.Fatalf("ClaudeAccount 必须新增 PersistentVolumeName 字段（D-01/D-02）")
	}
	if field.Type.Kind() != reflect.Ptr || field.Type.Elem().Kind() != reflect.String {
		t.Fatalf("PersistentVolumeName 必须是 *string 以承载 NULL 语义；实际为 %s", field.Type.String())
	}
	jsonTag := field.Tag.Get("json")
	if !strings.Contains(jsonTag, "persistent_volume_name") {
		t.Errorf("json tag 必须为 persistent_volume_name；实际为 %q", jsonTag)
	}
	if !strings.Contains(jsonTag, "omitempty") {
		t.Errorf("json tag 必须包含 omitempty，未分配时省略字段；实际为 %q", jsonTag)
	}

	// JSON 回路：未分配序列化应省略字段；已分配应序列化为字符串。
	unassigned := ClaudeAccount{ID: "a"}
	b, err := json.Marshal(unassigned)
	if err != nil {
		t.Fatalf("marshal unassigned: %v", err)
	}
	if strings.Contains(string(b), "persistent_volume_name") {
		t.Errorf("未分配（nil）时 JSON 不得出现 persistent_volume_name；实际：%s", string(b))
	}

	name := "claude-state-abc"
	assigned := ClaudeAccount{ID: "b", PersistentVolumeName: &name}
	b2, err := json.Marshal(assigned)
	if err != nil {
		t.Fatalf("marshal assigned: %v", err)
	}
	if !strings.Contains(string(b2), "\"persistent_volume_name\":\"claude-state-abc\"") {
		t.Errorf("已分配时 JSON 必须携带 persistent_volume_name；实际：%s", string(b2))
	}
}
