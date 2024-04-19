package fun

//	func Zero[T any]() T {
//		var a T
//		var t = reflect.TypeOf(a)
//		if t.Kind() == reflect.Ptr {
//			return reflect.New(t.Elem()).Interface().(T)
//		} else {
//			return a
//		}
//	}
func Zero[T any]() T {
	var a T
	return a
}
