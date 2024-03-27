package modelprotobuf

import "github.com/bep/execrpc"

var (
	_ execrpc.TagProvider          = &ExampleReceipt{}
	_ execrpc.LastModifiedProvider = &ExampleReceipt{}
	_ execrpc.SizeProvider         = &ExampleReceipt{}
)

func (r *ExampleReceipt) GetETag() string {
	return r.Identity.ETag
}

func (r *ExampleReceipt) SetETag(s string) {
	r.Identity.ETag = s
}

func (r *ExampleReceipt) GetELastModified() int64 {
	return r.Identity.LastModified
}

func (r *ExampleReceipt) SetELastModified(i int64) {
	r.Identity.LastModified = i
}

func (r *ExampleReceipt) GetESize() uint32 {
	return r.Identity.Size
}

func (r *ExampleReceipt) SetESize(i uint32) {
	r.Identity.Size = i
}
