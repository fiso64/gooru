from pathlib import Path

path = Path('docs/openapi.yaml')
text = path.read_text()
anchor = '  /search/suggestions:\n    get:\n'
post = '''  /search/suggestions:\n    post:\n      summary: Return search suggestions with free-form query state in the request body.\n      security:\n        - sessionAuth: []\n      requestBody:\n        required: true\n        content:\n          application/json:\n            schema:\n              $ref: "#/components/schemas/SuggestionRequest"\n      responses:\n        "200":\n          description: Search suggestions.\n          content:\n            application/json:\n              schema:\n                $ref: "#/components/schemas/SuggestionsResponse"\n        "400":\n          $ref: "#/components/responses/BadRequest"\n        "401":\n          $ref: "#/components/responses/Unauthorized"\n    get:\n'''
if anchor not in text:
    raise SystemExit('search suggestions OpenAPI anchor missing')
path.write_text(text.replace(anchor, post, 1))
