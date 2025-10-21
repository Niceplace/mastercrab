#! /opt/homebrew/bin/bash

DATE_FILTER="-P4D"
QUERY='{ "query": "query MyAssignedIssues { viewer { assignedIssues(filter: { updatedAt: { gte: \"'$DATE_FILTER'\" } }) { edges { node { id title url } } } } }", "operationName": "MyAssignedIssues" }'

response=$(curl --write-out "%{http_code}" --output ./responses/response.json \
  --request POST \
  --url https://api.linear.app/graphql \
  --header "Authorization: $LINEAR_MC_API_KEY" \
  --header "Content-Type: application/json" \
  --data "$QUERY")

if [ "$response" -eq 200 ]; then
  cat ./responses/response.json | jq .
else
  echo "Error: API request failed with status code $response"
  cat ./responses/response.json
fi

echo "Query is $QUERY"

#(filter: {updatedAt: {gte: \"$DATE_FILTER\"}})

# curl --request POST \
#     --header 'content-type: application/json' \
#     --url 'https://api.linear.app/graphql' \
#     --data '{"query":"query Query {\n  viewer {\n    assignedIssues(filter: { updatedAt: { gte: \"-PT24H\" } }) {\n      edges {\n        node {\n          id\n          title\n          url\n        }\n      }\n    }\n    createdIssues(filter: { updatedAt: { gte: \"-PT24H\" } }) {\n      edges {\n        node {\n          id\n          title\n          url\n        }\n      }\n    }\n  }\n}"}'