# MailKite for Go

Official [MailKite](https://mailkite.dev) SDK. One low-level `Request` plus one
method per endpoint. Standard library only. (Go exports methods in `PascalCase`;
the method set is identical to the other SDKs.)

## Install

```bash
go get github.com/fijiwebdesign/mailkite-go
```

## Usage

```go
package main

import (
	"fmt"
	"os"

	mailkite "github.com/fijiwebdesign/mailkite-go"
)

func main() {
	mk := mailkite.New(os.Getenv("MAILKITE_API_KEY"))

	res, err := mk.Send(mailkite.Message{
		From:    "hello@yourapp.mailkite.dev",
		To:      "ada@example.com",
		Subject: "Your invoice #1042",
		HTML:    "<p>Thanks! Receipt attached.</p>",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(res)
}
```

`Send` also accepts any `map`/struct. Point at a custom base URL with
`mailkite.NewWithBaseURL(key, url)`.

## Methods

`Send(message)`, `ListDomains()`, `CreateDomain(body)`, `GetDomain(id)`,
`DeleteDomain(id)`, `VerifyDomain(id)`, `SetWebhook(id, body)`,
`DeleteWebhook(id)`, `TestWebhook(id)`, `ListRoutes()`, `CreateRoute(body)`,
`ListMessages()`, `GetMessage(id)`, `RetryDelivery(id)`.

Non-2xx responses return a `*mailkite.Error` with `Status`, `Message`, `Body`.

See the [full docs](https://mailkite.dev/docs/libraries).
