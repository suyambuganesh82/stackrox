package file

import (
	"archive/zip"
	"io"
	"io/ioutil"
	"os"
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
				file = createFilePath(tt.modTime)
				defer os.Remove(file.path)
			} else {
				file = &File{path: "something that doesn't exist"}
			}
			gotModTime, err := file.ModTime()
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr, "ModTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.NoError(t, err, "OpenFromArchive() unexpected error = %v", err)
			if err != nil {
				t.Errorf("OpenFromArchive() unnexpected error = %v", err)
			}
			assert.True(t, tt.modTime.Equal(gotModTime), "ModTime() got = %v, expected = %v", gotModTime, tt.modTime)
		})
	}
}

func TestFile_OpenFromArchive(t *testing.T) {
	tests := []struct {
		name     string
		modTime  time.Time
		contents string
		fileName string
		wantErr  string
	}{
		{
			name:     "when zip archive contains the filename then extracts content with modification time",
			modTime:  time.Unix(123456789, 0),
			contents: "foo bar",
			fileName: "foobar.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zipFile := createZipFilePath(tt.modTime, tt.contents, tt.fileName)
			defer os.Remove(zipFile.Path())
			gotFile, gotModTime, err := zipFile.OpenFromArchive(tt.fileName)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr, "OpenFromArchive() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.NoError(t, err, "OpenFromArchive() unexpected error = %v", err)
			assert.True(t, tt.modTime.Equal(gotModTime), "OpenFromArchive() gotModTime = %v expected = %v", gotModTime, tt.modTime)
			gotContent, err := io.ReadAll(gotFile)
			assert.NoError(t, err, "reading file content failed")
			assert.Equal(t, string(gotContent), tt.contents, "OpenFromArchive() gotContents = %v expected = %v", gotContent, tt.contents)
		})
	}
}

func createTempOSFile() *os.File {
	tmpF, err := ioutil.TempFile(os.TempDir(), "TestFile_ModTime_*")
	utils.Must(err)
	return tmpF
}

func createFilePath(modTime time.Time) *File {
	tmpF := createTempOSFile()
	utils.Must(tmpF.Close())
	utils.Must(os.Chtimes(tmpF.Name(), modTime, modTime))
	return &File{
		path: tmpF.Name(),
	}
}

func createZipFilePath(modTime time.Time, contents, fileName string) *File {
	zipFile := createTempOSFile()
	zipWriter := zip.NewWriter(zipFile)

	fileWriter, err := zipWriter.Create(fileName)
	utils.Must(err)
	_, err = fileWriter.Write([]byte(contents))
	utils.Must(err)

	utils.Must(zipWriter.Close())
	utils.Must(zipFile.Close())
	utils.Must(os.Chtimes(zipFile.Name(), modTime, modTime))
	return &File{
		path: zipFile.Name(),
	}
}
