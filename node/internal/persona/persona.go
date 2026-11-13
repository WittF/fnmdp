package persona

import (
	"hash/fnv"

	"github.com/WittF/fnmdp/shared/httpx"
)

var pool = []httpx.Persona{
	chrome120Windows("120.0.6099.224"),
	chrome120Windows("120.0.6099.199"),
	chrome120Mac("120.0.6099.234"),
	chrome120Mac("120.0.6099.216"),
	chrome131Windows("131.0.6778.140"),
	chrome131Windows("131.0.6778.204"),
	chrome131Mac("131.0.6778.205"),
	chrome131Mac("131.0.6778.139"),
}

func ForNode(nodeID string) httpx.Persona {
	if len(pool) == 0 {
		return httpx.Persona{}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(nodeID))
	return pool[int(h.Sum32())%len(pool)]
}

func Pool() []httpx.Persona {
	return append([]httpx.Persona(nil), pool...)
}

func chrome120Windows(version string) httpx.Persona {
	return httpx.Persona{
		UA:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + " Safari/537.36",
		SecChUa:         `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
		SecChUaPlatform: `"Windows"`,
		ClientHello:     httpx.HelloChrome120,
	}
}

func chrome120Mac(version string) httpx.Persona {
	return httpx.Persona{
		UA:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + " Safari/537.36",
		SecChUa:         `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
		SecChUaPlatform: `"macOS"`,
		ClientHello:     httpx.HelloChrome120,
	}
}

func chrome131Windows(version string) httpx.Persona {
	return httpx.Persona{
		UA:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + " Safari/537.36",
		SecChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		SecChUaPlatform: `"Windows"`,
		ClientHello:     httpx.HelloChrome131,
	}
}

func chrome131Mac(version string) httpx.Persona {
	return httpx.Persona{
		UA:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + version + " Safari/537.36",
		SecChUa:         `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
		SecChUaPlatform: `"macOS"`,
		ClientHello:     httpx.HelloChrome131,
	}
}
