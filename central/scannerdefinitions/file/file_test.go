package file

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stackrox/rox/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestFile_ModTime(t *testing.T) {
	tests := []struct {
		name       string
		createFile bool
		modTime    time.Time
		wantErr    string
	}{
		{
			name:       "when file exists and a modification ime is set then returns the same modification time.",
			createFile: true,
			modTime:    time.Unix(123456789, 0),
		},
		{
			name:    "when file does not exist then return error",
			wantErr: "no such file or directory",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var file *File
			if tt.createFile {
				file = createFile(tt.modTime)
				defer os.Remove(file.path)
			} else {
				file = &File{path: "something that doesn't exist"}
			}
			modTime, err := file.ModTime()
			if (err == nil) != (tt.wantErr == "") {
				t.Errorf("ModTime() error = %v, wantErr %v", err, tt.wantErr)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			if !reflect.DeepEqual(modTime, tt.modTime) {
				t.Errorf("ModTime() got = %v, want %v", modTime, tt.modTime)
			}
		})
	}
}

func createFile(modTime time.Time) *File {
	tmpF, err := ioutil.TempFile(os.TempDir(), "TestFile_ModTime")
	utils.Must(err)
	utils.Must(tmpF.Close())
	utils.Must(os.Chtimes(tmpF.Name(), modTime, modTime))
	return &File{
		path: tmpF.Name(),
	}
}
