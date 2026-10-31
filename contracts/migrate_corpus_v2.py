"""Author v2 fixtures from historical contract inputs, never from a subject.

This is a harness maintenance tool, not a sidecar migration command. It rewrites
only contracts/goldens/v2 and contracts/corpus/v2.json. Legacy artifacts stay frozen.
"""
import base64
import copy
import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[1]
HEADER = '<!-- sidecar-format: 2 -->\n'
LIMITS = json.loads((ROOT / 'contracts/schema/runtime-v2.json').read_text())['limits']


def digest(*, data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()


def compact(*, value):
    return json.dumps(value, ensure_ascii=False, separators=(',', ':'))


def capture(*, location, source):
    logical = source.replace(b'\r\n', b'\n')
    lines = logical.split(b'\n')
    if logical.endswith(b'\n'):
        lines.pop()
    if not lines:
        lines = [b'']
    def offset(*, line, column):
        return sum(len(part) + 1 for part in lines[:line - 1]) + column - 1
    if location.startswith('line:'):
        line = int(location[5:])
        loc = {'kind':'line','line':line}
        start = offset(line=line, column=1)
        end = start + len(lines[line - 1])
    else:
        sl, sc, el, ec = map(int, re.fullmatch(r'range:(\d+):(\d+)-(\d+):(\d+)', location).groups())
        loc = {'kind':'range','start_line':sl,'start_column':sc,'end_line':el,'end_column':ec}
        start = offset(line=sl,column=sc)
        end = offset(line=el,column=ec)
    before = max(0, start - LIMITS['context_bytes'])
    after = min(len(logical), end + LIMITS['context_bytes'])
    while before < start and logical[before] & 0xc0 == 0x80:
        before += 1
    while after < len(logical) and logical[after] & 0xc0 == 0x80:
        after -= 1
    return {'version':1,'source_revision':digest(data=logical),'last_location':loc,
            'selected_text':logical[start:end].decode(),'prefix':logical[before:start].decode(),
            'suffix':logical[end:after].decode(),'prefix_at_start':before == 0,'suffix_at_end':after == len(logical)}


def document(*, data, source):
    if not data or data.isspace():
        return data
    text = data.decode('utf-8')
    if text.startswith('\ufeff'):
        return data
    ending = '\r\n' if '\r\n' in text else '\n'
    def opening(match):
        location = match[2]
        # Anchor assertions were removed. Retain selected bytes as historical evidence.
        location = location.split(' | ')[0]
        metadata = {}
        if location.startswith(('line:', 'range:')):
            metadata['tracking'] = capture(location=location, source=source)
        return match[1] + location + ']' + ending + '<!-- sidecar-tracking: ' + compact(value=metadata) + ' -->'
    text = re.sub(r'(\[@th_[^ ]+ )([^\]\r\n]+)\]', opening, text)
    return (HEADER.replace('\n', ending) + text).encode()


def migrate(*, fixture):
    fixture = copy.deepcopy(fixture)
    fixture['fixture_version'] = 2
    initial = fixture['initial_fs']['entries']
    sources = {entry['path']: base64.b64decode(entry.get('bytes_b64','')) for entry in initial if entry['kind'] == 'file'}
    if fixture['id'] in ('source-anchor-bad', 'source-bom', 'source-cr'):
        sources['a.txt'] = 'éx\nabc\n'.encode()
    # Existing line fixtures without a source intentionally exercise source-independent
    # operations. Their retained historical evidence is the original x+LF fixture.
    for fs in (fixture['initial_fs'], fixture['expect']['final_fs']):
        for entry in fs['entries']:
            if entry['kind'] != 'file':
                continue
            if entry['path'].endswith('.comments'):
                if 'bytes_template' in entry:
                    entry['bytes_template'] = document(data=entry['bytes_template'].encode(), source=sources.get(entry['path'][:-9], b'x\n')).decode()
                else:
                    entry['bytes_b64'] = base64.b64encode(document(data=base64.b64decode(entry.get('bytes_b64','')), source=sources.get(entry['path'][:-9], b'x\n'))).decode()
            elif entry['path'] == 'ops':
                raw = base64.b64decode(entry['bytes_b64'])
                if fixture['id'] in ('apply-protocol', 'apply-protocol-empty'):
                    raw = re.sub(rb'"protocol":\s*2', b'"protocol":1', raw)
                else:
                    raw = re.sub(rb'"protocol":\s*1', b'"protocol":2', raw)
                entry['bytes_b64'] = base64.b64encode(raw).decode()
    for stream in ('stdout','stderr'):
        template = stream + '_template'
        encoded = stream + '_b64'
        text = fixture['expect'].get(template)
        if text is None:
            text = base64.b64decode(fixture['expect'][encoded]).decode()
        text = text.replace('"protocol":1', '"protocol":2').replace('"format_version":1', '"format_version":2').replace('Format version: 1', 'Format version: 2').replace('Protocol version: 1', 'Protocol version: 2')
        # Repair diagnostics shift by the independently authored header/tracking prefix.
        text = re.sub(r'"byte_offset":(\d+)', lambda m:'"byte_offset":'+str(int(m[1])+len(HEADER.encode())+len('<!-- sidecar-tracking: {} -->\n')), text)
        text = re.sub(r'"line":2(?=,)', '"line":4', text)
        if template in fixture['expect']:
            fixture['expect'][template] = text
        else:
            fixture['expect'][encoded] = base64.b64encode(text.encode()).decode()
    fixture['stdin_b64'] = base64.b64encode(base64.b64decode(fixture['stdin_b64']).replace(b'"protocol":1', b'"protocol":2')).decode()
    if fixture['id'] == 'source-anchor-bad':
        # The obsolete source-anchor assertion is superseded by closed-format rejection.
        fixture['expect']['status'] = 3
        fixture['expect']['stderr_template'] = compact(value={'protocol':2,'error':{'code':'validation','message':'validation:target_content:location','details':{'path':'target_content','reason':'location'}}})+'\n'
        for fs in (fixture['initial_fs'], fixture['expect']['final_fs']):
            for entry in fs['entries']:
                if entry['path'].endswith('.comments'):
                    data = base64.b64decode(entry['bytes_b64']).replace(b'range:1:1-1:3]', b'range:1:1-1:3 | w6k=]')
                    entry['bytes_b64'] = base64.b64encode(data).decode()
    return fixture


def main():
    legacy = json.loads((ROOT / 'contracts/corpus/v1.json').read_text())
    current = json.loads((ROOT / 'contracts/corpus/v2.json').read_text())
    legacy_ids = {fixture['id'] for fixture in legacy['fixtures']} - {'version-json'}
    current['fixtures'] = [fixture for fixture in current['fixtures'] if fixture['id'] not in legacy_ids]
    for ref in legacy['fixtures']:
        if ref['id'] == 'version-json':
            continue
        try:
            fixture = migrate(fixture=json.loads((ROOT / ref['golden']).read_text()))
        except Exception as error:
            raise RuntimeError(f"migration failed for {ref['id']}") from error
        new_ref = copy.deepcopy(ref)
        new_ref['golden'] = ref['golden'].replace('/v1/', '/v2/')
        (ROOT / new_ref['golden']).write_text(json.dumps(fixture, ensure_ascii=False, indent=2)+'\n')
        current['fixtures'].append(new_ref)
    current['requirements'].update(legacy['requirements'])
    (ROOT / 'contracts/corpus/v2.json').write_text(json.dumps(current, indent=2)+'\n')


if __name__ == '__main__':
    main()
