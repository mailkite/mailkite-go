<p align="center">
  <a href="https://mailkite.dev">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://mailkite.dev/brand/logo-email-dark.png">
      <img src="https://mailkite.dev/brand/logo-email.png" alt="MailKite" height="56">
    </picture>
  </a>
</p>

<h1 align="center">MailKite for Go</h1>

<p align="center">
  <b>Email for every product you ship</b> — receive email as a webhook, send over a verified domain, give an AI agent its own inbox.
  <br>The official <a href="https://mailkite.dev">MailKite</a> library for Go.
</p>

<p align="center">
  <a href="https://mailkite.dev/docs">Docs</a> ·
  <a href="https://mailkite.dev/docs/libraries">Library guide</a> ·
  <a href="https://mailkite.dev">mailkite.dev</a> ·
  <a href="https://mailkite.dev/docs/ai-agents">AI agents</a>
</p>

> **Read-only mirror.** This repo is a generated, release-time mirror of the MailKite monorepo (the private source of truth) — development doesn't happen here. Install from Go modules and open issues against the [MailKite docs](https://mailkite.dev/docs).

## Install

```bash
go get github.com/mailkite/mailkite-go
```

## Quickstart

```go
package main

import (
	"fmt"
	"os"

	mailkite "github.com/mailkite/mailkite-go"
)

func main() {
	mk := mailkite.New(os.Getenv("MAILKITE_API_KEY"))

	res, err := mk.Send(mailkite.Message{
		From:    "hello@myapp.ai",
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

## Examples

Runnable examples live in [`examples/`](examples/) — send mail, verify webhooks, build an AI email agent, and log users in:

| Example | What it shows |
| --- | --- |
| [`examples/agent-inbox`](examples/agent-inbox) | Give your agent its own email address — MailKite's built-in inbox agent answers mail for… |
| [`examples/agent-reply`](examples/agent-reply) | An AI email agent: inbound email → Claude composes a reply → MailKite sends it, threaded. |
| [`examples/send`](examples/send) | Send an email over a verified domain — the 10-second "it works". |
| [`examples/server-login`](examples/server-login) | Server-side login + register — your own account, or your users' accounts via OAuth 2.1 +… |
| [`examples/webhook`](examples/webhook) | Receive inbound email as a webhook — and VERIFY the HMAC signature before trusting it. |

## API methods

Every method is documented on its own page under [`docs/`](docs/). The full surface:

| Method | What it does |
| --- | --- |
| [`send`](docs/send.md) | Send a message over a verified domain. Pass `templateId` (+ optional `templateData`) to… |
| [`uploadAttachment`](docs/uploadAttachment.md) | Upload a file to MailKite storage and get back a secure, time-limited URL. Reference the… |
| [`listTemplates`](docs/listTemplates.md) | List your saved email templates (light metadata only — no body). Use getTemplate for the… |
| [`listBaseTemplates`](docs/listBaseTemplates.md) | List the premade base templates (light metadata). Clone one with createTemplate({ baseId… |
| [`getTemplate`](docs/getTemplate.md) | Get one template (full: subject, html, text, theme). Works for your templates (tpl_…) and… |
| [`createTemplate`](docs/createTemplate.md) | Create a template. Pass `baseId` to clone a base template into your own, or provide… |
| [`listDomains`](docs/listDomains.md) | List your domains, each with its webhook URL. |
| [`createDomain`](docs/createDomain.md) | Add a domain. Returns the domain + DNS records. |
| [`getDomain`](docs/getDomain.md) | Get one domain with DNS records + webhook. |
| [`deleteDomain`](docs/deleteDomain.md) | Remove a domain. |
| [`verifyDomain`](docs/verifyDomain.md) | Check DNS and update status. |
| [`setWebhook`](docs/setWebhook.md) | Set or replace the domain's catch-all webhook. |
| [`deleteWebhook`](docs/deleteWebhook.md) | Remove the domain's webhook. |
| [`testWebhook`](docs/testWebhook.md) | Send a signed test event to the domain's webhook. |
| [`checkDomainAvailability`](docs/checkDomainAvailability.md) | Check whether a domain is available to register, and at what price. Read-only — no charge. |
| [`registerDomain`](docs/registerDomain.md) | Register (buy) a domain on the customer's behalf; provisions mail DNS and adds it to the… |
| [`listRoutes`](docs/listRoutes.md) | List inbound routing rules. |
| [`createRoute`](docs/createRoute.md) | Create a route (match, action, destination). |
| [`agent`](docs/agent.md) | Send a message to one of your inbox agents and get its reply. Defaults to the account's… |
| [`route`](docs/route.md) | Route a message to one of your registered routes (by `routeId` or `address`), running… |
| [`listMessages`](docs/listMessages.md) | List stored messages, newest first. Optionally page with `before` (a `received_at`… |
| [`getMessage`](docs/getMessage.md) | Get a message with deliveries + attachments. |
| [`retryDelivery`](docs/retryDelivery.md) | Re-deliver a stored message to its webhook. |
| [`listLists`](docs/listLists.md) | List your contact lists (static, curated broadcast audiences), each with its member count. |
| [`createList`](docs/createList.md) | Create a contact list. Returns the list with its id (lst_…); add contacts with… |
| [`getList`](docs/getList.md) | Get one contact list with its member count. |
| [`updateList`](docs/updateList.md) | Rename a contact list. |
| [`deleteList`](docs/deleteList.md) | Delete a contact list. The list is removed; the contacts themselves are kept. |
| [`listListContacts`](docs/listListContacts.md) | List the contacts that are members of a list, newest first. Optionally page with `before`… |
| [`addListContacts`](docs/addListContacts.md) | Add contacts (by id, ctr_…) to a list. Returns how many were newly added; contacts… |
| [`removeListContact`](docs/removeListContact.md) | Remove one contact from a list (the contact itself is kept). |
| [`listBroadcasts`](docs/listBroadcasts.md) | List your broadcasts (one-to-many sends) with status and send stats. |
| [`createBroadcast`](docs/createBroadcast.md) | Create a broadcast draft. `from` is required; set `audience` to { type: "all" } or {… |
| [`getBroadcast`](docs/getBroadcast.md) | Get one broadcast with its status and recipient summary. |
| [`updateBroadcast`](docs/updateBroadcast.md) | Edit a draft broadcast (any of from/subject/audience/html/… ). Drafts only. |
| [`deleteBroadcast`](docs/deleteBroadcast.md) | Delete a broadcast draft. |
| [`sendBroadcast`](docs/sendBroadcast.md) | Send a broadcast now, or pass an ISO 8601 `scheduledAt` to schedule it. A one-click… |
| [`verifyWebhook`](docs/verifyWebhook.md) | Verify the `x-mailkite-signature` header on an inbound webhook delivery. Runs entirely… |
| [`replyOk`](docs/replyOk.md) | The acknowledgement body a webhook consumer returns to confirm it processed the event —… |
| [`replySpam`](docs/replySpam.md) | Control-mode reply a webhook consumer returns to tell MailKite to mark the message as… |
| [`replyDrop`](docs/replyDrop.md) | Control-mode reply a webhook consumer returns to tell MailKite to drop (discard) the… |
| [`replyBlockSender`](docs/replyBlockSender.md) | Control-mode reply a webhook consumer returns to tell MailKite to block the sender — the… |
| [`encrypt`](docs/encrypt.md) | Encrypt a UTF-8 string to a domain's RSA public key (SPKI/PEM), returning the at-rest… |
| [`decrypt`](docs/decrypt.md) | Decrypt a MailKite at-rest envelope JSON with your RSA private key (PKCS8/PEM), returning… |
| [`semanticSearch`](docs/semanticSearch.md) | Semantic search over the MailKite documentation — returns the most relevant doc sections… |

## Use it from an AI agent — MCP + Agent connectors

MailKite speaks the [Model Context Protocol](https://modelcontextprotocol.io): every API method is a tool your AI assistant (Claude, Cursor, …) can call — send mail, manage domains, search the docs, and give an agent its own inbox. Full guide: **[https://mailkite.dev/docs/ai-agents](https://mailkite.dev/docs/ai-agents)**.

**Hosted (recommended) — one-click OAuth, no key to copy:**

```bash
claude mcp add --transport http mailkite https://mcp.mailkite.dev/mcp
```

In Claude Code you can also install the plugin:

```text
/plugin marketplace add mailkite/claude-code
/plugin install mailkite@mailkite
```

Any chat/UI agent: *"Add the MCP server at https://mcp.mailkite.dev/mcp and authenticate in the browser when prompted."*

**Local (static key, offline / CI):**

```json
{ "mcpServers": { "mailkite": { "command": "npx", "args": ["-y", "@mailkite/mcp"], "env": { "MAILKITE_API_KEY": "mk_live_…" } } } }
```

**Give an agent its own inbox.** Route inbound mail to a built-in **inbox agent** (the `agent` route action) and it answers, files, or escalates on its own — see [https://mailkite.dev/docs/ai-agents](https://mailkite.dev/docs/ai-agents).

## All MailKite libraries

Same contract, every language — pick the one for your stack (full list: [https://mailkite.dev/docs/libraries](https://mailkite.dev/docs/libraries)):

| Library | Repo | Distribution |
| --- | --- | --- |
| MailKite for Node.js | [`mailkite-node`](https://github.com/mailkite/mailkite-node) | npm |
| MailKite for Python | [`mailkite-python`](https://github.com/mailkite/mailkite-python) | PyPI |
| MailKite for Ruby | [`mailkite-ruby`](https://github.com/mailkite/mailkite-ruby) | RubyGems |
| MailKite for Java | [`mailkite-java`](https://github.com/mailkite/mailkite-java) | Maven Central |
| MailKite for PHP | [`mailkite-php`](https://github.com/mailkite/mailkite-php) | Packagist |
| MailKite for Go **(this repo)** | [`mailkite-go`](https://github.com/mailkite/mailkite-go) | Go modules |
| @mailkite/cli | [`mailkite-cli`](https://github.com/mailkite/mailkite-cli) | npm |
| @mailkite/mcp | [`mailkite-mcp`](https://github.com/mailkite/mailkite-mcp) | npm |
| @mailkite/client | [`mailkite-js`](https://github.com/mailkite/mailkite-js) | npm |
| @mailkite/expo | [`mailkite-expo`](https://github.com/mailkite/mailkite-expo) | npm |
| MailKiteClient | [`mailkite-swift`](https://github.com/mailkite/mailkite-swift) | Swift Package Manager |
| dev.mailkite:mailkite-client | [`mailkite-kotlin`](https://github.com/mailkite/mailkite-kotlin) | Maven Central |
| mailkite_client | [`mailkite-flutter`](https://github.com/mailkite/mailkite-flutter) | pub.dev |

## Docs & links

- 📚 **Documentation:** https://mailkite.dev/docs
- 📦 **This library's guide:** https://mailkite.dev/docs/libraries
- 🤖 **AI agents (MCP + inbox agents):** https://mailkite.dev/docs/ai-agents
- 🌐 **Website:** https://mailkite.dev
- 🧭 **All libraries:** https://mailkite.dev/docs/libraries

<sub>Generated from the shared MailKite API contract. © MailKite.</sub>
