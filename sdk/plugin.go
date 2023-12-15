/*
 *    Copyright 2023 [lihan aooohan@gmail.com]
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package sdk

import (
	"fmt"
	"github.com/aooohan/version-fox/env"
	"github.com/aooohan/version-fox/lua_module"
	"github.com/aooohan/version-fox/util"
	lua "github.com/yuin/gopher-lua"
)

const (
	LuaPluginObjKey = "PLUGIN"
	OsType          = "OS_TYPE"
	ArchType        = "ARCH_TYPE"
	PluginVersion   = "0.0.1"
)

type LuaPlugin struct {
	state     *lua.LState
	pluginObj *lua.LTable
	Name      string
	Author    string
	Version   string
	UpdateUrl string
}

func (l *LuaPlugin) checkValid() error {
	if l.state == nil {
		return fmt.Errorf("lua vm is nil")
	}
	obj := l.pluginObj
	if obj.RawGetString("Available") == lua.LNil {
		return fmt.Errorf("[Available] function not found")
	}
	if obj.RawGetString("PostInstall") == lua.LNil {
		return fmt.Errorf("[PostInstall] function not found")
	}
	if obj.RawGetString("EnvKeys") == lua.LNil {
		return fmt.Errorf("[EnvKeys] function not found")
	}
	return nil
}

func (l *LuaPlugin) Close() {
	l.state.Close()
}

func (l *LuaPlugin) Available() ([]*Package, error) {
	L := l.state
	ctxTable := L.NewTable()
	L.SetField(ctxTable, "plugin_version", lua.LString(PluginVersion))
	if err := L.CallByParam(lua.P{
		Fn:      l.pluginObj.RawGetString("Available").(*lua.LFunction),
		NRet:    1,
		Protect: true,
	}, l.pluginObj, ctxTable); err != nil {
		return nil, err
	}

	table := L.ToTable(-1) // returned value
	L.Pop(1)               // remove received value

	if table.Type() == lua.LTNil {
		return []*Package{}, nil
	}
	var err error
	var result []*Package
	table.ForEach(func(key lua.LValue, value lua.LValue) {
		kvTable, ok := value.(*lua.LTable)
		if !ok {
			err = fmt.Errorf("the return value is not a table")
			return
		}
		v := kvTable.RawGetString("version").String()
		note := kvTable.RawGetString("note").String()
		mainSdk := &Info{
			Name:    l.Name,
			Version: Version(v),
			Note:    note,
		}
		var additionalArr []*Info
		additional := kvTable.RawGetString("additional")
		if tb, ok := additional.(*lua.LTable); ok && tb.Len() != 0 {
			additional.(*lua.LTable).ForEach(func(key lua.LValue, value lua.LValue) {
				itemTable, ok := value.(*lua.LTable)
				if !ok {
					err = fmt.Errorf("the return value is not a table")
					return
				}
				item := Info{
					Name:    itemTable.RawGetString("name").String(),
					Version: Version(itemTable.RawGetString("version").String()),
				}
				additionalArr = append(additionalArr, &item)
			})
		}

		result = append(result, &Package{
			Main:       mainSdk,
			Additional: additionalArr,
		})

	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (l *LuaPlugin) PreInstall(version Version) (*Package, error) {
	L := l.state
	ctxTable := L.NewTable()
	L.SetField(ctxTable, "version", lua.LString(version))

	if err := L.CallByParam(lua.P{
		Fn:      l.pluginObj.RawGetString("PostInstall").(*lua.LFunction),
		NRet:    1,
		Protect: true,
	}, l.pluginObj, ctxTable); err != nil {
		return nil, err
	}

	table := L.ToTable(-1) // returned value
	L.Pop(1)               // remove received value
	if table.Type() == lua.LTNil {
		return nil, nil
	}
	v := table.RawGetString("version").String()
	muStr := table.RawGetString("url").String()
	mainSdk := &Info{
		Name:    l.Name,
		Version: Version(v),
		Path:    muStr,
	}
	var additionalArr []*Info
	additional := table.RawGetString("additional")
	if tb, ok := additional.(*lua.LTable); ok && tb.Len() != 0 {
		var err error
		additional.(*lua.LTable).ForEach(func(key lua.LValue, value lua.LValue) {
			kvTable, ok := value.(*lua.LTable)
			if !ok {
				err = fmt.Errorf("the return value is not a table")
				return
			}
			s := kvTable.RawGetString("url").String()
			item := Info{
				Name:    kvTable.RawGetString("name").String(),
				Version: Version(kvTable.RawGetString("version").String()),
				Path:    s,
			}
			additionalArr = append(additionalArr, &item)
		})
		if err != nil {
			return nil, err
		}
	}

	return &Package{
		Main:       mainSdk,
		Additional: additionalArr,
	}, nil
}

func (l *LuaPlugin) PostInstall(rootPath string, sdks []*Info) error {
	L := l.state
	sdkArr := L.NewTable()
	for _, v := range sdks {
		sdkTable := L.NewTable()
		L.SetField(sdkTable, "name", lua.LString(v.Name))
		L.SetField(sdkTable, "version", lua.LString(v.Version))
		L.SetField(sdkTable, "path", lua.LString(v.Path))
		L.SetField(sdkArr, v.Name, sdkTable)
	}
	ctxTable := L.NewTable()
	L.SetField(ctxTable, "sdkInfo", sdkArr)
	L.SetField(ctxTable, "rootPath", lua.LString(rootPath))

	if err := L.CallByParam(lua.P{
		Fn:      l.pluginObj.RawGetString("PostInstall").(*lua.LFunction),
		NRet:    1,
		Protect: true,
	}, l.pluginObj, ctxTable); err != nil {
		return err
	}

	return nil
}

func (l *LuaPlugin) EnvKeys(sdkPackage *Package) ([]*env.KV, error) {
	L := l.state
	ctxTable := L.NewTable()
	L.SetField(ctxTable, "path", lua.LString(sdkPackage.Main.Path))
	if len(sdkPackage.Additional) != 0 {
		additionalTable := L.NewTable()
		for _, v := range sdkPackage.Additional {
			L.SetField(additionalTable, v.Name, lua.LString(v.Path))
		}
		L.SetField(ctxTable, "additional_path", additionalTable)
	}
	if err := L.CallByParam(lua.P{
		Fn:      l.pluginObj.RawGetString("EnvKeys"),
		NRet:    1,
		Protect: true,
	}, l.pluginObj, ctxTable); err != nil {
		return nil, err
	}

	table := L.ToTable(-1) // returned value
	L.Pop(1)               // remove received value
	if table.Type() == lua.LTNil || table.Len() == 0 {
		return nil, fmt.Errorf("no environment variables provided")
	}
	var err error
	var envKeys []*env.KV
	table.ForEach(func(key lua.LValue, value lua.LValue) {
		kvTable, ok := value.(*lua.LTable)
		if !ok {
			err = fmt.Errorf("the return value is not a table")
			return
		}
		key = kvTable.RawGetString("key")
		value = kvTable.RawGetString("value")
		envKeys = append(envKeys, &env.KV{Key: key.String(), Value: value.String()})
	})
	if err != nil {
		return nil, err
	}

	return envKeys, nil
}

func (l *LuaPlugin) getTableField(table *lua.LTable, fieldName string) (lua.LValue, error) {
	value := table.RawGetString(fieldName)
	if value.Type() == lua.LTNil {
		return nil, fmt.Errorf("field '%s' not found", fieldName)
	}
	return value, nil
}

func (l *LuaPlugin) luaPrint() int {
	L := l.state
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		for i := 1; i <= top; i++ {
			fmt.Print(L.ToStringMeta(L.Get(i)))
			if i != top {
				fmt.Print("\t")
			}
		}
		fmt.Println()
		return 0
	}))
	return 0
}

func (l *LuaPlugin) Label(version string) string {
	return fmt.Sprintf("%s@%s", l.Name, version)
}

func NewLuaPlugin(content string, osType util.OSType, archType util.ArchType) (*LuaPlugin, error) {
	L := lua.NewState()
	lua_module.Preload(L)
	if err := L.DoString(content); err != nil {
		return nil, fmt.Errorf("content cannot be executed")
	}

	// set OS_TYPE and ARCH_TYPE
	L.SetGlobal(OsType, lua.LString(osType))
	L.SetGlobal(ArchType, lua.LString(archType))

	pluginOjb := L.GetGlobal(LuaPluginObjKey)
	if pluginOjb.Type() == lua.LTNil {
		return nil, fmt.Errorf("plugin object not found")
	}

	PLUGIN := pluginOjb.(*lua.LTable)

	source := &LuaPlugin{
		state:     L,
		pluginObj: PLUGIN,
	}

	if err := source.checkValid(); err != nil {
		return nil, err
	}

	if name := PLUGIN.RawGetString("name"); name.Type() != lua.LTNil {
		source.Name = name.String()
	}
	if version := PLUGIN.RawGetString("version"); version.Type() != lua.LTNil {
		source.Version = version.String()
	}
	if updateUrl := PLUGIN.RawGetString("updateUrl"); updateUrl.Type() != lua.LTNil {
		source.UpdateUrl = updateUrl.String()
	}
	if author := PLUGIN.RawGetString("author"); author.Type() != lua.LTNil {
		source.Author = author.String()
	}
	return source, nil
}
