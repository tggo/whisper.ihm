module whisper.ihm

go 1.23

require (
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0-00010101000000-000000000000
	github.com/hajimehoshi/go-mp3 v0.3.4
	github.com/oov/audio v0.0.0-20171004131523-88a2be6dbe38
)

replace github.com/ggerganov/whisper.cpp/bindings/go => ./whisper.cpp/bindings/go
