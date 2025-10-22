#! /opt/homebrew/bin/bash

DATE_FILTER="-P1D"
QUERY='{ "query": "query MyAssignedIssues { viewer { assignedIssues(filter: { updatedAt: { gte: \"'$DATE_FILTER'\" } }) { edges { node { id title url } } } } }", "operationName": "MyAssignedIssues" }'
OUTPUT_FILE="response.json"

response=$(curl --write-out "%{http_code}" --output "$OUTPUT_FILE" \
  --request POST \
  --url https://api.linear.app/graphql \
  --header "Authorization: $MC_LINEAR_API_KEY" \
  --header "Content-Type: application/json" \
  --data "$QUERY")

if [ "$response" -eq 200 ]; then
  echo "Successerooo !"
else
  echo "Error: API request failed with status code $response"
  
fi
cat "$OUTPUT_FILE" | jq .
echo "Query is $QUERY"

#(filter: {updatedAt: {gte: \"$DATE_FILTER\"}})

# curl --request POST \
#     --header 'content-type: application/json' \
#     --url 'https://api.linear.app/graphql' \
#     --data '{"query":"query Query {\n  viewer {\n    assignedIssues(filter: { updatedAt: { gte: \"-PT24H\" } }) {\n      edges {\n        node {\n          id\n          title\n          url\n        }\n      }\n    }\n    createdIssues(filter: { updatedAt: { gte: \"-PT24H\" } }) {\n      edges {\n        node {\n          id\n          title\n          url\n        }\n      }\n    }\n  }\n}"}'