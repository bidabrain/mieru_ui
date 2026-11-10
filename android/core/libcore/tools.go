//go:build tools

package libcore

// Keeps golang.org/x/mobile/bind in go.mod, as required by `gomobile bind`.
import _ "golang.org/x/mobile/bind"
