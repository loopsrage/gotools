package index

import (
	"fmt"
	"testing"
)

func TestIndex_StoreInIndex(t *testing.T) {
	idx := NewIndex()
	type args struct {
		indexName string
		key       interface{}
		value     interface{}
	}
	tests := []struct {
		name string
		args []args
	}{
		{
			name: "",
			args: []args{
				{
					indexName: "People_Ages",
					key:       "Korbyn",
					value:     8,
				},
				{
					indexName: "People_Ages",
					key:       "Kaliopi",
					value:     9,
				},
				{
					indexName: "People_Ages",
					key:       "Kambrya",
					value:     6,
				},
				{
					indexName: "House Numbers",
					key:       "Home",
					value:     2121,
				},
				{
					indexName: "House Numbers",
					key:       "Other",
					value:     2022,
				},
				{
					indexName: "Animals",
					key:       "type",
					value:     "zebra",
				},
				{
					indexName: "People_Ages",
					key:       "Korbyn",
					value:     13,
				},
				{
					indexName: "Names",
					key:       "Cooper",
					value:     13,
				},
				{
					indexName: "Soup",
					key:       "Chimney",
					value:     13,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, v := range tt.args {
				idx.StoreInIndex(v.indexName, v.key, v.value)
			}
			for _, v := range tt.args {
				val, err := idx.LoadFromIndex(v.indexName, v.key)
				if err != nil {
					t.Fatal(err)
				}

				if val == nil {
					t.Fatal(val)
				}
			}
		})
	}

	for _, v := range idx.ListIndexes() {
		err := idx.RangeIndex(v, func(key, value interface{}) {
			fmt.Println(v, key, value)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
