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
