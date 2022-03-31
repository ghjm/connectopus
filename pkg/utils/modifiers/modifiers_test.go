package modifiers

import "testing"

type modsTest struct {
	mod1 string
	mod2 string
}

func mod1(s string) func(mt *modsTest) {
	return func(mt *modsTest) {
		mt.mod1 = s
	}
}

func mod2(s string) func(mt *modsTest) {
	return func(mt *modsTest) {
		mt.mod2 = s
	}
}

func testMods(mods... func(*modsTest)) *modsTest {
	mt := &modsTest{}
	ProcessMods(mt, mods)
	return mt
}

func TestProcessMods(t *testing.T) {
	mt := testMods(mod1("foo"), mod2("bar"))
	if mt.mod1 != "foo" || mt.mod2 != "bar" {
		t.Error("mods did not return expected results")
	}
}
