package file_cleaner

import (
	"os"
	"testing"
)

func TestFileCleaner(t *testing.T) {
	temp1, err := os.CreateTemp("", "file_cleaner_test")
	if err != nil {
		t.Fatal(err)
	}
	err = temp1.Close()
	if err != nil {
		t.Fatal(err)
	}
	DeleteOnExit(temp1.Name())
	var temp2 *os.File
	temp2, err = os.CreateTemp("", "file_cleaner_test")
	if err != nil {
		t.Fatal(err)
	}
	err = temp2.Close()
	if err != nil {
		t.Fatal(err)
	}
	DeleteOnExit(temp2.Name())
	DeleteAllNow()
	_, err = os.Stat(temp1.Name())
	if !os.IsNotExist(err) {
		t.Fatalf("%s should not exist", temp1.Name())
	}
	_, err = os.Stat(temp2.Name())
	if !os.IsNotExist(err) {
		t.Fatalf("%s should not exist", temp2.Name())
	}
}
