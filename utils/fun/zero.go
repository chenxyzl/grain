package fun

import "reflect"

func Zero[T any]() T {
	var a T
	var t = reflect.TypeOf(a)
	if t.Kind() == reflect.Ptr {
		return reflect.New(t.Elem()).Interface().(T)
	} else {
		return a
	}
}
