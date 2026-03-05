Notion Cli
---

TODO

create a notion-cli to search/read pages on Notion

primary used by llm harness (aka ai agent, like claude code, opencode etc.)

write in python

commands:


**notion**

print help message


**notion configure**

check config is there, if exist, prompt to confirm reconfigure


prompt to input notion_secret. write it to $HOME/.config/notion-cli/config.json


**notion search [--sort-timestamp last_edited_time] [--sort-direction descending] [--start-cursor uuid] [--page-size n] (query)***

equivalent:

```bash
curl --request POST \
  --url https://api.notion.com/v1/search \
  --header 'Authorization: Bearer <token>' \
  --header 'Content-Type: application/json' \
  --header 'Notion-Version: 2025-09-03' \
  --data '
{
  "sort": {
    "timestamp": "last_edited_time",
    "direction": "descending"
  },
  "query": "<string>",
  "start_cursor": "3c90c3cc-0d44-4b50-8888-8dd25736052a",
  "page_size": 10
}
'
```

**notion fetch-page (page_id)**

equivalent:

```bash
curl --request GET \
  --url https://api.notion.com/v1/pages/{page_id}/markdown \
  --header 'Authorization: Bearer <token>' \
  --header 'Notion-Version: 2025-09-03'
```


using inline script metadata to specify dependency


put `#!/usr/bin/env -S uv run --script` as the first line, to auto resolve dependency


finally update README.md , write a proper introduction
