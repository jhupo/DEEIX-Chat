package billing

// Module 聚合计费 HTTP 处理器。
type Module struct {
	Handler *Sub2Handler
}

func NewSub2Module(handler *Sub2Handler) *Module { return &Module{Handler: handler} }

// NewModule 创建计费 HTTP 模块。

func NewModule(handler *Sub2Handler) *Module {
	return &Module{Handler: handler}
}
