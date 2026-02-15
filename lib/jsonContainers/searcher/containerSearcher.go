package searcher

import "golangApp/lib/jsonContainers/container"

type ContainerSearcher interface {
	Current(container *container.Container)
}
