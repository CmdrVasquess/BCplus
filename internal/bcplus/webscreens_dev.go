// +build !release

package bcplus

import (
	"github.com/CmdrVasquess/bcplus/internal/wapp/proto"
	_ "github.com/CmdrVasquess/bcplus/internal/wapp/travel"
)

var stdScreenTabOrder = []string{proto.Key}
