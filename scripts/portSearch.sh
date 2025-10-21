curl --request POST \
  --url https://api.getport.io/v1/blueprints/_team/entities/search \
  --header "Authorization: Bearer $PORT_MC_TOKEN" \
  --header 'Content-Type: application/json' \
  --data '{
	"include": [
		"$identifier",
		"$title"
	],
	"query": {
		"combinator": "and",
		"rules": [
			{
				"property": "$title",
				"operator": "=",
				"value": "team-devx"
			}
		]
	},
	"limit": 1000
}'