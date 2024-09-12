package xlog

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
)

func addFileToZip(srcFile string, zipWriter *zip.Writer) error {
	src, err := os.Open(srcFile)
	if err != nil {
		fmt.Errorf("%v", err)
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		fmt.Errorf("%v", err)
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		fmt.Errorf("%v", err)
		return err
	}
	header.Name = path.Base(srcFile)
	header.SetMode(0666)
	if !info.IsDir() {
		header.Method = zip.Deflate
	}

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		fmt.Errorf("%v", err)
		return err
	}

	_, err = io.Copy(writer, src)
	if err != nil {
		fmt.Errorf("%v", err)
		return err
	}
	return nil
}
