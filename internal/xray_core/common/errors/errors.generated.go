package errors

type errPathObjHolder struct{}

func NewError(values ...interface{}) *Error {
	return New(values...).WithPathObj(errPathObjHolder{})
}
