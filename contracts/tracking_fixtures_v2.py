"""Independently authored tracking/editor contract examples (no subject execution)."""
import base64
import copy
import json
from migrate_corpus_v2 import ROOT, HEADER, capture, compact, digest

THREAD = '018f0000-0000-7000-8000-000000000001'
MESSAGE = '018f0000-0000-7000-8000-000000000101'
SOURCE = b'private before\nselected\nprivate after\n'
LOCATION = {'kind':'line','line':2}
RECORD = capture(location='line:2', source=SOURCE)
MESSAGES = [{'id':MESSAGE,'author':{'kind':'human'},'body':'discussion survives'}]


def file(*, path, data):
    return {'path':path,'kind':'file','bytes_b64':base64.b64encode(data).decode(),'mode':'0600'}


def stored(*, location):
    return (HEADER + '[@th_' + THREAD + ' ' + location + ']\n<!-- sidecar-tracking: ' + compact(value={'tracking':RECORD}) + ' -->\n---user <!-- sidecar-meta: ' + compact(value={'id':MESSAGE}) + ' -->\ndiscussion survives\n[/@th_' + THREAD + ']\n').encode()


def main():
    manifest_path = ROOT / 'contracts/corpus/v2.json'
    manifest = json.loads(manifest_path.read_text())
    manifest['requirements']['V2-TRACKING'] = {'source':'docs/v2/format.md','subject':'retained tracking, explicit excerpt reads, unresolved editor deletion and privacy'}
    initial = [file(path='draft',data=SOURCE),file(path='draft.comments',data=stored(location='line:2'))]
    storage = '{{sha256:initial:draft.comments}}'
    summary = {key:value for key,value in RECORD.items() if key not in ('selected_text','prefix','suffix')}
    for name, include in [('tracking-redacted',False),('tracking-explicit',True)]:
        thread = {'id':THREAD,'location':LOCATION,'messages':MESSAGES,'tracking':RECORD if include else summary}
        argv = ['thread','get','--file','draft.comments','--id',THREAD]
        if include:
            argv.append('--include-tracking')
        save(manifest=manifest,name=name,argv=argv,initial=initial,final=initial,result={'thread':thread,'storage_revision':storage,'canonical_revision':storage},command='thread-get',changed=False)
    outcome = {'thread_id':THREAD,'outcome':'unchanged','reason':'source_unchanged','evidence':'unchanged','location':LOCATION}
    for check in (True,False):
        final = copy.deepcopy(initial)
        if not check:
            final.append(file(path='.draft.comments.sidecar.lock',data=b''))
        save(manifest=manifest,name='tracking-sync-check' if check else 'tracking-sync-noop',argv=['sync','--file','draft.comments']+(['--check'] if check else []),initial=initial,final=final,command='sync',changed=False,result={'check':check,'would_change':False,'source_revision':RECORD['source_revision'],'previous_storage_revision':storage,'storage_revision':storage,'canonical_revision':storage,'outcomes':[outcome]})
    request = {'protocol':2,'if_revision':digest(data=stored(location='line:2')),'source_revision':RECORD['source_revision'],'updates':[{'thread_id':THREAD,'deleted':True}]}
    editor_initial = initial + [file(path='updates',data=compact(value=request).encode())]
    final = [file(path='draft',data=SOURCE),file(path='draft.comments',data=stored(location='unresolved')),editor_initial[-1],file(path='.draft.comments.sidecar.lock',data=b'')]
    save(manifest=manifest,name='editor-deleted',argv=['editor-update','--file','draft.comments','--operations-file','updates'],initial=editor_initial,final=final,command='editor-update',changed=True,result={'source_revision':RECORD['source_revision'],'storage_revision':'{{sha256:final:draft.comments}}','canonical_revision':'{{sha256:final:draft.comments}}','outcomes':[{'thread_id':THREAD,'outcome':'unresolved','reason':'editor_deleted','evidence':'editor','location':{'kind':'unresolved'}}]})
    request['updates'] = [{'thread_id':THREAD,'location':LOCATION}]
    mapped_initial = initial + [file(path='updates',data=compact(value=request).encode())]
    mapped_final = mapped_initial + [file(path='.draft.comments.sidecar.lock',data=b'')]
    save(manifest=manifest,name='editor-location-noop',argv=['editor-update','--file','draft.comments','--operations-file','updates'],initial=mapped_initial,final=mapped_final,command='editor-update',changed=False,result={'source_revision':RECORD['source_revision'],'storage_revision':storage,'canonical_revision':storage,'outcomes':[{'thread_id':THREAD,'outcome':'unchanged','reason':'editor_location','evidence':'editor','location':LOCATION}]})
    manifest_path.write_text(json.dumps(manifest,indent=2)+'\n')


def save(*, manifest, name, argv, initial, final, command, changed, result):
    golden = {'fixture_version':2,'id':name,'requirements':['V2-TRACKING'],'argv':['--output','json']+argv,'env':[],'stdin_b64':'','requires':['writer_platform'] if command in ('sync','editor-update') and '--check' not in argv else [],'initial_fs':{'entries':initial},'expect':{'status':0,'stdout_template':compact(value={'protocol':2,'command':command,'changed':changed,'result':result})+'\n','stderr_b64':'','final_fs':{'entries':final}}}
    path = 'contracts/goldens/v2/'+name+'.json'
    (ROOT/path).write_text(json.dumps(golden,ensure_ascii=False,indent=2)+'\n')
    manifest['fixtures'] = [ref for ref in manifest['fixtures'] if ref['id'] != name]
    manifest['fixtures'].append({'id':name,'golden':path,'requirements':['V2-TRACKING'],'coverage':['BHV-V2-'+name.upper()]})


if __name__ == '__main__':
    main()
