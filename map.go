package ws_connector

import "sync"

type Map interface {
	Exists(key interface{}) bool
	Load(key interface{}) interface{}
	Store(key, value interface{})
	Update(key interface{}, updateFunc func(oldValue interface{}) interface{}) interface{}
}

func NewMap() Map {
	return &wscMap{m: make(map[interface{}]interface{})}
}

type wscMap struct {
	m map[interface{}]interface{}
	l sync.RWMutex
}

func (m *wscMap) Exists(key interface{}) bool {
	m.l.RLock()
	defer m.l.RUnlock()
	_, e := m.m[key]
	return e
}

func (m *wscMap) Load(key interface{}) interface{} {
	m.l.RLock()
	defer m.l.RUnlock()
	return m.m[key]
}

func (m *wscMap) Store(key, value interface{}) {
	m.l.Lock()
	defer m.l.Unlock()
	m.m[key] = value
}

// Update returns the new value (which is returned by the specified function)
func (m *wscMap) Update(key interface{}, updateFunc func(oldValue interface{}) interface{}) interface{} {
	m.l.Lock()
	defer m.l.Unlock()
	newValue := updateFunc(m.m[key])
	m.m[key] = newValue
	return newValue
}

func (m *wscMap) Delete(key interface{}) {
	m.l.Lock()
	defer m.l.Unlock()
	delete(m.m, key)
}
