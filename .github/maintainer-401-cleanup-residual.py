from pathlib import Path
import re

serve_path = Path('cmd/gooru/cmd/serve.go')
text = serve_path.read_text(encoding='utf-8')
old = '\t\t\t"jobs_max_queued", cfg.Jobs.MaxQueued,\n\t\t\t"jobs_max_running", cfg.Jobs.MaxRunning,\n'
if old not in text:
    raise RuntimeError('expected legacy jobs runtime log fields')
text = text.replace(old, '\t\t\t"uploads_max_queued", cfg.Uploads.MaxQueued,\n')
serve_path.write_text(text, encoding='utf-8')

config_test_path = Path('internal/serve/config_test.go')
text = config_test_path.read_text(encoding='utf-8')
pattern = re.compile(r'\nfunc TestLoadConfigRejectsInvalidJobLimits\([^\n]*\)[^{]*\{.*?(?=\nfunc |\Z)', re.S)
text, count = pattern.subn('\n', text, count=1)
if count != 1:
    raise RuntimeError(f'expected invalid job limits test once, found {count}')
config_test_path.write_text(text, encoding='utf-8')

openapi_test_path = Path('internal/serve/openapi_test.go')
text = openapi_test_path.read_text(encoding='utf-8')
old_block = '''\n\tjobSchema := schema(t, spec, "Job")
\tjobRequired := stringSlice(t, jobSchema["required"])
\tif !containsString(jobRequired, "submitted_at") {
\t\tt.Fatalf("Job schema must require submitted_at, got %+v", jobRequired)
\t}
\tjobProps := stringMap(t, jobSchema["properties"])
\tfor _, field := range []string{"started_at", "finished_at", "result", "error"} {
\t\tif _, ok := jobProps[field]; !ok {
\t\t\tt.Fatalf("Job schema missing %q", field)
\t\t}
\t}
'''
new_block = '''\n\toperationSchema := schema(t, spec, "BackgroundOperation")
\toperationRequired := stringSlice(t, operationSchema["required"])
\tfor _, field := range []string{"id", "kind", "status", "progress_total", "progress_completed", "progress_failed", "created_at"} {
\t\tif !containsString(operationRequired, field) {
\t\t\tt.Fatalf("BackgroundOperation schema missing required %q in %+v", field, operationRequired)
\t\t}
\t}
\toperationProps := stringMap(t, operationSchema["properties"])
\tfor _, field := range []string{"started_at", "finished_at", "result", "error_code", "error_message"} {
\t\tif _, ok := operationProps[field]; !ok {
\t\t\tt.Fatalf("BackgroundOperation schema missing %q", field)
\t\t}
\t}
'''
if old_block not in text:
    raise RuntimeError('expected legacy Job OpenAPI assertions')
text = text.replace(old_block, new_block)
old_assert = '\tassertResponseSchemaRef(t, spec, "/jobs", "get", "200", "#/components/schemas/JobListResponse")\n'
new_assert = '\tassertResponseSchemaRef(t, spec, "/operations", "get", "200", "#/components/schemas/BackgroundOperationListResponse")\n\tassertResponseSchemaRef(t, spec, "/operations/{id}", "get", "200", "#/components/schemas/BackgroundOperation")\n'
if old_assert not in text:
    raise RuntimeError('expected legacy /jobs OpenAPI assertion')
text = text.replace(old_assert, new_assert)
openapi_test_path.write_text(text, encoding='utf-8')
