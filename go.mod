module clearlink

go 1.26.0

require (
	github.com/bwmarrin/discordgo v0.27.1
	github.com/hraban/opus v0.0.0-20251117090126-c76ea7e21bf3
	github.com/jpoirier/gortlsdr v2.10.0+incompatible
	github.com/warthog618/go-gpiocdev v0.9.1
	gopkg.in/ini.v1 v1.67.1
)

require (
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
)

replace github.com/bwmarrin/discordgo => github.com/yeongaori/discordgo-fork v0.0.0-20260319072544-e8e546f5d532
