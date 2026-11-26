package files

import "testing"

func TestUploadNamesCannotEscapeDirectory(test *testing.T) {
	for _, name := range []string{"", ".", "..", "../secret", "/tmp/file", `C:\file`, "nested/file", "nul\x00file"} {
		if ValidateName(name) == nil {
			test.Errorf("accepted unsafe filename %q", name)
		}
	}
	for _, name := range []string{"中文文件.txt", "has spaces.md", "quote'\".txt", "-n"} {
		if err := ValidateName(name); err != nil {
			test.Errorf("rejected safe name %q: %v", name, err)
		}
	}
}

func TestPathValidationRejectsInvalidUTF8(test *testing.T) {
	for _, filename := range []string{"", "/nul\x00", string([]byte{255})} {
		if ValidatePath(filename) == nil {
			test.Errorf("accepted invalid path %q", filename)
		}
	}
	if err := ValidatePath("/home/中文/a b.txt"); err != nil {
		test.Fatal(err)
	}
}
