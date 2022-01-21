代码分为两部分：

typecheck1: noder.go sizes.go import.go: syntax -> ir.Node + types.Type

typecheck2: 把type2的结果转换为typecheck1结果
