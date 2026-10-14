package relayconvert

import relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"

type MediaResolver = relaymedia.MediaResolver
type GeminiMedia = relaymedia.GeminiMedia

func SetMediaResolver(resolver MediaResolver) {
	relaymedia.SetMediaResolver(resolver)
}
