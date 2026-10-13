package constant

const (
	HappyOysterAdventure = "happyoyster-1.0-adventure"
	HappyOysterDirecting = "happyoyster-1.0-directing"
)

func HappyOysterModel(name string) string {
	if name == "" {
		return HappyOysterAdventure
	}
	return name
}

func HappyOysterAPIPath(name string) string {
	return "/api/v2/apps/" + HappyOysterModel(name) + "/openapi/v1"
}

func HappyOysterClientScope(name, path string) string {
	switch path {
	case "/worlds/build-status":
		return "world:build-status"
	case "/travels/enter-travel":
		return "travel:enter"
	case "/travels/status":
		return "travel:status"
	case "/travels/end":
		return "travel:end"
	}
	if name == HappyOysterDirecting {
		switch path {
		case "/travels/rewind", "/travels/pause", "/travels/resume", "/travels/instruct", "/travels/update-script":
			return "travel:" + path[len("/travels/"):]
		}
	}
	return ""
}
