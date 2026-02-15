package catchPanic

func CatchPanic(f func(), cb func(interface{})) {
	defer func() {
		switch err := recover().(type) {
		case error:
			cb(err)
		default:
			cb(err)
		}
	}()
	f()
}
