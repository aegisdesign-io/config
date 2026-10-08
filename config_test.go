package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aegisdesign-io/assert"
	"go.yaml.in/yaml/v4"
)

func TestDecodeErrorWrapsCause(t *testing.T) {
	var syn *json.SyntaxError
	err := fromJson([]byte(`{bad`), new(testConfig))
	assert.With(t).
		That(errors.As(err, &syn)).
		IsTrue()
}

// Test helpers mirroring FromFile without reading a file.
func fromJson(buf []byte, konf interface{}) error {
	if err := checkTarget(konf); err != nil {
		return err
	}
	return decode(buf, konf, json.Unmarshal)
}

func fromYaml(buf []byte, konf interface{}) error {
	if err := checkTarget(konf); err != nil {
		return err
	}
	return decode(buf, konf, yaml.Unmarshal)
}

type basicList []interface{}

type basicMap struct {
	Integer int     `json:"integer" yaml:"integer"`
	Float   float64 `json:"float" yaml:"float"`
	String  string  `json:"string" yaml:"string"`
	Boolean bool    `json:"boolean" yaml:"boolean"`
}

type testMap struct {
	basicMap `yaml:",inline"`
	Sublist  basicList `json:"list" yaml:"list"`
	Submap   basicMap  `json:"map" yaml:"map"`
}

type testConfig struct {
	Map testMap `json:"map" yaml:"map"`
}

func TestFromJsonFile(t *testing.T) {
	var k = new(testConfig)
	var err = FromFile("config_test.json", k)
	assert.With(t).
		That(err).
		IsOk()
	validate(k, t, float64(1234))
}

func TestFromYamlFile(t *testing.T) {
	var k = new(testConfig)
	var err = FromFile("config_test.yaml", k)
	assert.With(t).
		That(err).
		IsOk()
	validate(k, t, 1234)
}

func TestWithEnvOverride(t *testing.T) {
	// Nested structure to test.
	type testStruct struct {
		Str    string `json:"str" yaml:"str"`
		Num    int    `json:"num" env:"NUM"`
		Nested struct {
			Str string `json:"str" yaml:"str"`
			Num int    `json:"num" env:"NUM"`
		}
		PtrNested *struct {
			Num int `json:"num" env:"NUM"`
		} `json:"ptrNested" yaml:"ptrNested"`
		Unsigned uint16 `env:"NUM"`
		Ptr      *int   `env:"NUM"`
	}

	jsonString := `{"str":"foobar","num":-1111,"nested":{"str":"deadbeef","num":-2222},"ptrNested":{"num":-3333}}`
	ts := new(testStruct)
	t.Setenv("NUM", "65535")
	err := fromJson([]byte(jsonString), ts)

	Assert := assert.With(t)
	Assert.
		That(err).
		IsOk()
	Assert.
		That(ts.Num).
		IsEqualTo(65535)
	Assert.
		That(ts.Nested.Num).
		IsEqualTo(65535)
	Assert.
		That(ts.PtrNested.Num).
		IsEqualTo(65535)
	Assert.
		That(ts.Unsigned).
		IsEqualTo(uint16(65535))
	Assert.
		That(*ts.Ptr).
		IsEqualTo(65535)
}

func TestEnvOverflow(t *testing.T) {
	t.Setenv("PORT", "70000")
	type s struct {
		Port int16 `env:"PORT"`
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err).
		IsNotNil()
}

func TestEnvParseError(t *testing.T) {
	t.Setenv("PORT", "abc")
	type s struct {
		Port int `env:"PORT"`
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "PORT")).
		IsEqualTo(true)
}

func TestEnvUnexportedField(t *testing.T) {
	t.Setenv("PORT", "80")
	type s struct {
		port int `env:"PORT"`
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err).
		IsNotNil()
}

func TestInvalidTarget(t *testing.T) {
	Assert := assert.With(t)
	Assert.
		That(FromFile("config_example.yaml", AppConfig{})).
		IsNotNil()
	Assert.
		That(FromFile("config_example.yaml", (*AppConfig)(nil))).
		IsNotNil()
}

func TestJsonTrailingData(t *testing.T) {
	err := fromJson([]byte(`{} garbage`), new(testConfig))
	assert.With(t).
		That(err).
		IsNotNil()
}

func TestFromFileMissing(t *testing.T) {
	err := FromFile("does_not_exist.yaml", new(testConfig))
	assert.With(t).
		That(err).
		IsNotNil()
}

func TestFromFileUnknownExtension(t *testing.T) {
	err := FromFile("LICENSE", new(testConfig))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "unexpected file format")).
		IsTrue()
}

func TestJsonInvalidTarget(t *testing.T) {
	err := fromJson([]byte(`{}`), testConfig{})
	assert.With(t).
		That(err).
		IsNotNil()
}

func TestYamlDecodeError(t *testing.T) {
	err := fromYaml([]byte("map: [unclosed"), new(testConfig))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "yaml")).
		IsTrue()
}

func TestEnvNestedError(t *testing.T) {
	t.Setenv("PORT", "abc")
	err := FromFile("config_example.yaml", new(AppConfig))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "PORT")).
		IsTrue()
}

func TestEnvUnsetKeepsFileValue(t *testing.T) {
	t.Setenv("CONFIG_TEST_UNSET", "")
	os.Unsetenv("CONFIG_TEST_UNSET") // t.Setenv restores the original value
	type s struct {
		Name string `json:"name" env:"CONFIG_TEST_UNSET"`
	}
	k := new(s)
	err := fromJson([]byte(`{"name":"from-file"}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(k.Name).
		IsEqualTo("from-file")
}

func TestEnvNilPointerStruct(t *testing.T) {
	t.Setenv("CONFIG_TEST_DB_PORT", "5")
	type s struct {
		Db *struct {
			Port int `env:"CONFIG_TEST_DB_PORT"`
		}
		Cache *struct {
			Port int `env:"CONFIG_TEST_CACHE_PORT"`
		}
	}
	k := new(s)
	err := fromJson([]byte(`{}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(k.Db != nil && k.Db.Port == 5).
		IsTrue()
	Assert.
		That(k.Cache == nil).
		IsTrue()
}

func TestEnvEmptyOverridesStringOnly(t *testing.T) {
	t.Setenv("CONFIG_TEST_EMPTY_PORT", "")
	t.Setenv("CONFIG_TEST_EMPTY_NAME", "")
	type s struct {
		Port int     `json:"port" env:"CONFIG_TEST_EMPTY_PORT"`
		Name string  `json:"name" env:"CONFIG_TEST_EMPTY_NAME"`
		Ptr  *string `json:"ptr" env:"CONFIG_TEST_EMPTY_NAME"`
	}
	k := new(s)
	err := fromJson([]byte(`{"port":80,"name":"from-file","ptr":"from-file"}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(k.Port == 80 && k.Name == "" && k.Ptr != nil && *k.Ptr == "").
		IsTrue()
}

func TestEnvDurationAndTextUnmarshaler(t *testing.T) {
	t.Setenv("CONFIG_TEST_TIMEOUT", "30s")
	t.Setenv("CONFIG_TEST_IP", "10.0.0.1")
	type s struct {
		Timeout time.Duration `env:"CONFIG_TEST_TIMEOUT"`
		IP      net.IP        `env:"CONFIG_TEST_IP"`
	}
	k := new(s)
	err := fromJson([]byte(`{}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(k.Timeout == 30*time.Second && k.IP.Equal(net.ParseIP("10.0.0.1"))).
		IsTrue()
}

func TestSetFromString(t *testing.T) {
	tests := []struct {
		name    string
		target  interface{}
		val     string
		want    interface{}
		wantErr bool
	}{
		{"int", new(int), "-42", -42, false},
		{"int bad", new(int), "abc", nil, true},
		{"int8 overflow", new(int8), "200", nil, true},
		{"uint", new(uint), "42", uint(42), false},
		{"uint8 overflow", new(uint8), "300", nil, true},
		{"uint negative", new(uint32), "-1", nil, true},
		{"bool", new(bool), "true", true, false},
		{"bool bad", new(bool), "maybe", nil, true},
		{"float32", new(float32), "1.5", float32(1.5), false},
		{"float64", new(float64), "3.14159", 3.14159, false},
		{"float bad", new(float64), "pi", nil, true},
		{"string", new(string), "hello", "hello", false},
		{"pointer", new(*int), "7", 7, false},
		{"pointer bad", new(*int), "seven", nil, true},
		{"unsupported", new([]string), "a,b", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := reflect.ValueOf(tt.target).Elem()
			err := setFromString(field, tt.val)
			Assert := assert.With(t)
			if tt.wantErr {
				Assert.
					That(err).
					IsNotNil()
				return
			}
			Assert.
				That(err).
				IsNil()
			if field.Kind() == reflect.Ptr {
				field = field.Elem()
			}
			Assert.
				That(field.Interface()).
				IsEqualTo(tt.want)
		})
	}
}
// wantInt is the decoded type of an integer in a list: float64 for JSON, int
// for YAML.
func validate(k *testConfig, t *testing.T, wantInt any) {
	Assert := assert.With(t)
	Assert.
		That(k.Map.Boolean).
		IsEqualTo(true)
	Assert.
		That(k.Map.Float).
		IsEqualTo(3.14159)
	Assert.
		That(k.Map.Integer).
		IsEqualTo(1234)
	Assert.
		That(k.Map.String).
		IsEqualTo("string")
	Assert.
		That(len(k.Map.Sublist)).
		IsEqualTo(4)
	Assert.
		That(k.Map.Sublist[0]).
		IsEqualTo(true)
	Assert.
		That(k.Map.Sublist[1]).
		IsEqualTo("string")
	Assert.
		That(k.Map.Sublist[2]).
		IsEqualTo(3.14159)
	Assert.
		That(k.Map.Sublist[3]).
		IsEqualTo(wantInt)
	Assert.
		That(k.Map.Submap.Boolean).
		IsEqualTo(true)
	Assert.
		That(k.Map.Submap.Float).
		IsEqualTo(3.14159)
	Assert.
		That(k.Map.Submap.Integer).
		IsEqualTo(1234)
	Assert.
		That(k.Map.Submap.String).
		IsEqualTo("string")
}

// Example structure that matches config_example.yaml.
// NOTE: no struct tags are needed because every YAML key is the lowercased
// field name. Otherwise, add `yaml:"..."` tags; YAML ignores json tags.
type AppConfig struct {
	Server struct {
		Port uint16 `env:"PORT"`
	}
	Database struct {
		Driver   string
		Hostname string
		Port     uint16
		Username string
		Password string
		Name     string
	}
}

// Reads the AppConfig data from config_example.yaml and prints it.
func ExampleFromFile() {
	if old, ok := os.LookupEnv("PORT"); ok {
		defer os.Setenv("PORT", old)
	} else {
		defer os.Unsetenv("PORT")
	}
	os.Setenv("PORT", "9000")
	var c = new(AppConfig)
	if err := FromFile("config_example.yaml", c); err != nil {
		fmt.Printf("Error: %v", err)
	}

	fmt.Printf("%+v\n", *c)
	// OUTPUT: {Server:{Port:9000} Database:{Driver:postgres Hostname:localhost Port:5432 Username:postgres Password:dummy Name:my_database}}
}

type envTestNode struct {
	Name string `env:"CONFIG_TEST_NODE_NAME"`
	Next *envTestNode
}

type envTestPlainNode struct {
	Name string
	Next *envTestPlainNode
}

func TestEnvSelfReferentialType(t *testing.T) {
	t.Setenv("CONFIG_TEST_NODE_NAME", "head")
	type s struct {
		Plain *envTestPlainNode
		Head  envTestNode
	}
	k := new(s)
	err := fromJson([]byte(`{"head":{"next":{}}}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(k.Plain == nil && k.Head.Name == "head" && k.Head.Next.Name == "head" && k.Head.Next.Next == nil).
		IsTrue()
}

func TestEnvPointerCycle(t *testing.T) {
	t.Setenv("CONFIG_TEST_NODE_NAME", "loop")
	k := &struct{ Head *envTestNode }{Head: &envTestNode{}}
	k.Head.Next = k.Head
	Assert := assert.With(t)
	Assert.
		That(fromEnvironment(k)).
		IsNil()
	Assert.
		That(k.Head.Name).
		IsEqualTo("loop")
}

func TestEnvUnexportedNestedStructError(t *testing.T) {
	t.Setenv("CONFIG_TEST_DB_PORT", "5432")
	type s struct {
		db struct {
			Port int `env:"CONFIG_TEST_DB_PORT"`
		}
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "db")).
		IsTrue()
}

type envTestTimeout time.Duration

func (d *envTestTimeout) UnmarshalText(b []byte) error {
	x, err := time.ParseDuration(string(b))
	*d = envTestTimeout(x)
	return err
}

type envTestUserID int64

func TestEnvNamedInt64RejectsDuration(t *testing.T) {
	t.Setenv("CONFIG_TEST_USER", "30s")
	type s struct {
		User envTestUserID `env:"CONFIG_TEST_USER"`
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err != nil && !strings.Contains(err.Error(), "duration")).
		IsTrue()
}

func TestEnvSharedAddressNotSkipped(t *testing.T) {
	t.Setenv("CONFIG_TEST_OUTER", "set")
	type inner struct {
		A string `env:"CONFIG_TEST_INNER"`
	}
	type outer struct {
		In inner
		B  string `env:"CONFIG_TEST_OUTER"`
	}
	type s struct {
		I *inner
		O *outer
	}
	o := new(outer)
	k := &s{I: &o.In, O: o}
	Assert := assert.With(t)
	Assert.
		That(fromEnvironment(k)).
		IsNil()
	Assert.
		That(o.B).
		IsEqualTo("set")
}

func TestEnvUnexportedNestedStructUnset(t *testing.T) {
	type s struct {
		db struct {
			Port int `env:"CONFIG_TEST_UNSET_PORT"`
		}
	}
	assert.With(t).
		That(fromJson([]byte(`{}`), new(s))).
		IsNil()
}

func TestEnvNamedDuration(t *testing.T) {
	t.Setenv("CONFIG_TEST_TIMEOUT", "30s")
	type s struct {
		Timeout envTestTimeout `env:"CONFIG_TEST_TIMEOUT"`
	}
	k := new(s)
	err := fromJson([]byte(`{}`), k)
	Assert := assert.With(t)
	Assert.
		That(err).
		IsNil()
	Assert.
		That(time.Duration(k.Timeout)).
		IsEqualTo(30 * time.Second)
}

func TestFromFileInvalidTargetBeforeRead(t *testing.T) {
	err := FromFile("does_not_exist.yaml", testConfig{})
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "target must be")).
		IsTrue()
}

func TestEnvDurationAcceptsNanoseconds(t *testing.T) {
	t.Setenv("CONFIG_TEST_TIMEOUT", "1500")
	type s struct {
		Timeout time.Duration `env:"CONFIG_TEST_TIMEOUT"`
	}
	k := new(s)
	Assert := assert.With(t)
	Assert.
		That(fromJson([]byte(`{}`), k)).
		IsNil()
	Assert.
		That(k.Timeout).
		IsEqualTo(1500 * time.Nanosecond)
}

type envTestUNode struct {
	Name string `env:"CONFIG_TEST_NODE_NAME"`
	next *envTestUNode
}

func TestEnvUnexportedSelfReferentialPointer(t *testing.T) {
	type s struct {
		Head envTestUNode
	}
	assert.With(t).
		That(fromJson([]byte(`{}`), new(s))).
		IsNil()
}

func TestEnvDeepUnexportedStructError(t *testing.T) {
	t.Setenv("CONFIG_TEST_DEEP_PORT", "5")
	type inner struct {
		Port int `env:"CONFIG_TEST_DEEP_PORT"`
	}
	type mid struct {
		in inner
	}
	type s struct {
		Mid mid
	}
	err := fromJson([]byte(`{}`), new(s))
	assert.With(t).
		That(err != nil && strings.Contains(err.Error(), "Mid.in.Port")).
		IsTrue()
}

func TestEnvPointerWritesThrough(t *testing.T) {
	t.Setenv("CONFIG_TEST_SHARED", "9")
	type s struct {
		A *int `env:"CONFIG_TEST_SHARED"`
		B *int
	}
	shared := 1
	k := &s{A: &shared, B: &shared}
	Assert := assert.With(t)
	Assert.
		That(fromEnvironment(k)).
		IsNil()
	Assert.
		That(*k.A == 9 && *k.B == 9).
		IsTrue()
}

type envTestLevel string

func (l *envTestLevel) UnmarshalText(b []byte) error {
	if string(b) != "debug" && string(b) != "info" {
		return fmt.Errorf("unknown level %q", b)
	}
	*l = envTestLevel(b)
	return nil
}

func TestEnvEmptySkipsTextUnmarshalerString(t *testing.T) {
	t.Setenv("CONFIG_TEST_LEVEL", "")
	type s struct {
		Level envTestLevel `json:"level" env:"CONFIG_TEST_LEVEL"`
	}
	k := new(s)
	Assert := assert.With(t)
	Assert.
		That(fromJson([]byte(`{"level":"info"}`), k)).
		IsNil()
	Assert.
		That(k.Level).
		IsEqualTo(envTestLevel("info"))
}
