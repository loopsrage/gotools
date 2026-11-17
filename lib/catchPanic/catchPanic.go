package catchPanic

func CatchPanic(f func(), cb func(interface{})) {
	defer func() {
		if err := recover(); err != nil {
			cb(err)
		}
	}()
	f()
}
