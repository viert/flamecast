package icy

import (
	"testing"
)

var (
	input1    []byte
	expected1 = MetaData{"StreamTitle": "Some test string"}

	input2    []byte
	expected2 = MetaData{"StreamTitle": "Test string with 'apostrophes'"}

	input3 []byte
)

func prepareData() {
	input1 = append([]byte{2}, []byte("StreamTitle='Some test string';")...)
	input1 = append(input1, 0)

	input2 = append([]byte{3}, []byte("StreamTitle='Test string with 'apostrophes'';")...)
	input2 = append(input2, 0, 0, 0)

	input3 = append([]byte{3}, []byte("StreamTitle='Test string which ends unexpecte")...)
	input3 = append(input3, 0, 0, 0)
}

func TestParseMeta(t *testing.T) {

	prepareData()
	var md MetaFrame

	md = MetaFrame(input1)
	output1, err := md.ParseMeta()
	if err != nil {
		t.Errorf("error while parsing valid data: %s", err)
	}
	for k, v := range output1 {
		if expected1[k] != v {
			t.Errorf("unexpected value by key %s: expected %s but got %s", k, expected1[k], v)
		}
	}

	md = MetaFrame(input2)
	output2, err := md.ParseMeta()
	if err != nil {
		t.Errorf("error while parsing valid data: %s", err)
	}
	for k, v := range output2 {
		if expected2[k] != v {
			t.Errorf("unexpected value by key %s: expected %s but got %s", k, expected2[k], v)
		}
	}

}

func TestParseEmpty(t *testing.T) {
	md := make(MetaFrame, 0)
	_, err := md.ParseMeta()
	if err == nil {
		t.Error("should return error on empty metaframe")
	}
}

func TestParseInvalid(t *testing.T) {
	prepareData()
	md := MetaFrame(input3)
	_, err := md.ParseMeta()
	if err == nil {
		t.Error("should return error on unexpectedly ending metaframe")
	}
}
