package searcher

import "github.com/loopsrage/gotools/lib/jsonContainers/container"

type ContainerSearcher interface {
	Current(container *container.Container)
}
