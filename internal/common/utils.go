package common

import (
	"archive/tar"
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

func ReadToml[T any](tomlPath string) (T, error) {
	var model T
	_, err := toml.DecodeFile(tomlPath, &model)
	if err != nil {
		return model, err
	}
	return model, nil
}

func CreateTar(path string) ([]byte, error) {
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)

	defer tw.Close()

	err := filepath.WalkDir(path, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(path, file)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
