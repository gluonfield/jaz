package filepathx

import "testing"

func TestFileURI(t *testing.T) {
	tests := []struct {
		name string
		goos string
		path string
		want string
	}{
		{name: "posix", goos: "linux", path: "/tmp/a b.txt", want: "file:///tmp/a%20b.txt"},
		{name: "windows drive", goos: "windows", path: `C:\Users\wins\a b.txt`, want: "file:///C:/Users/wins/a%20b.txt"},
		{name: "windows unc", goos: "windows", path: `\\server\share\a b.txt`, want: "file://server/share/a%20b.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileURI(tt.path, tt.goos); got != tt.want {
				t.Fatalf("fileURI(%q, %q) = %q, want %q", tt.path, tt.goos, got, tt.want)
			}
		})
	}
}

func TestFileURLToPath(t *testing.T) {
	tests := []struct {
		name string
		goos string
		raw  string
		want string
	}{
		{name: "posix", goos: "linux", raw: "file:///tmp/a%20b.txt", want: "/tmp/a b.txt"},
		{name: "posix host", goos: "linux", raw: "file://server/share/a%20b.txt", want: "//server/share/a b.txt"},
		{name: "windows drive", goos: "windows", raw: "file:///C:/Users/wins/a%20b.txt", want: `C:\Users\wins\a b.txt`},
		{name: "windows localhost", goos: "windows", raw: "file://localhost/C:/Users/wins/a%20b.txt", want: `C:\Users\wins\a b.txt`},
		{name: "windows unc", goos: "windows", raw: "file://server/share/a%20b.txt", want: `\\server\share\a b.txt`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fileURLToPath(tt.raw, tt.goos)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("fileURLToPath(%q, %q) = %q, want %q", tt.raw, tt.goos, got, tt.want)
			}
		})
	}
}
