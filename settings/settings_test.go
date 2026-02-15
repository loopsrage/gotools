package settings

import (
	"fmt"
	"testing"
)

func TestRestoreFromFile(t *testing.T) {

	const pth = "/home/jarek/GolandProjects/uploader-server-lib-v3/serverInterfaces/remoteFilesServer/remoteFilesServices/s3index/settings/exampleSettings/.env"
	t.Run("RestoreFromFile", func(t *testing.T) {
		err := RestoreFromFile(pth)
		if err != nil {
			t.Fatal(err)
		}

		settingsContainer.RangeValues(func(key string, value interface{}) {
			fmt.Println(key, value)
		})
	})
}
