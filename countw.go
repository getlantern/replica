package replica

type CountWriter struct {
	BytesWritten int64
}

func (me *CountWriter) Write(b []byte) (int, error) {
	me.BytesWritten += int64(len(b))
	return len(b), nil
}
