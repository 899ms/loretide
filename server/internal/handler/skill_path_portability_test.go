package handler

import "testing"

func TestValidateFilePathPortable(t *testing.T) {
	for _, value := range []string{"/abs/SKILL.md", `\abs\SKILL.md`, `C:\abs\SKILL.md`, "C:/abs/SKILL.md", "C:SKILL.md", `\\server\share\SKILL.md`, "//server/share/SKILL.md", `..\escape\SKILL.md`, "../escape/SKILL.md", "file:stream", "bad\x00name"} {
		t.Run(value, func(t *testing.T) {
			if validateFilePath(value) {
				t.Errorf("accepted unsafe portable path %q", value)
			}
		})
	}
	for _, value := range []string{"SKILL.md", "scripts/run.sh", "references/说明.md"} {
		if !validateFilePath(value) {
			t.Errorf("rejected relative path %q", value)
		}
	}
}
