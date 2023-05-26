package logreport

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}

func TestParseLogLTSV(t *testing.T) {
	buf := []byte("String:string\tInt:1000\tFloat:11.3245")
	columns := []logColumn{
		{Name: "String", DataType: DataTypeString},
		{Name: "Int", DataType: DataTypeInt},
		{Name: "Float", DataType: DataTypeFloat},
	}
	log, err := ParseLog(buf, columns, "ltsv")
	if err != nil {
		t.Fatalf("ParseLog Error %+v", err)
	}
	if log.String("String") != "string" {
		t.Fatalf("String Error %s", log.String("String"))
	}
	if log.Int("Int") != 1000 {
		t.Fatalf("Int Error %d", log.Int("Int"))
	}
	if log.Float("Float") != 11.3245 {
		t.Fatalf("Float Error %f", log.Float("Float"))
	}
	if !log.IsColumn("String") {
		t.Fatal("IsColumn String not found")
	}
	if log.IsColumn("Hoge") {
		t.Fatal("IsColumn Hoge found")
	}
}
