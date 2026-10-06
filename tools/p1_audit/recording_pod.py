#!/usr/bin/env python3
"""Opt-in local recording profile pod; never uses host ports or existing services.

Requires Go and cached postgres:17-alpine, redis:7-alpine,
go-recorder-minio:local, rabbitmq:4.2.9-management, go-recorder-worker images.
The worker image must contain FFmpeg and be Linux arm64.
"""
import argparse, datetime, hashlib, json, pathlib, secrets, subprocess, time, threading, os

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--phase', choices=['before', 'after'], default='after')
parser.add_argument('--baseline-ref', default='86bded3', help='before overlays only composite/capture.go from this Git ref')
parser.add_argument('--output', type=pathlib.Path, help='new evidence directory; must not exist')
parser.add_argument('--rabbit', action='store_true', help='also profile confirmed RabbitMQ commands and reconnect')
args = parser.parse_args()

root=pathlib.Path(__file__).resolve().parents[2]
phase=args.phase
stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d-%H%M%S')
base=(args.output or root/'tmp/p1-audit'/f'{stamp}-{phase}-{secrets.token_hex(3)}').resolve()
base.mkdir(parents=True, exist_ok=False)
evidence=base/'recording'
rabbit_evidence=base/'rabbitmq'
evidence.mkdir(); rabbit_evidence.mkdir()
tag='p1-audit-recording-'+secrets.token_hex(4)
label_key='recorder.p1.audit'
label=label_key+'='+tag
network=tag+'-net'
names={role:tag+'-'+role for role in ['pod','postgres','minio','rabbit','test','rabbit-test']}
created=[]
volumes=[]
net_created=False
password=secrets.token_urlsafe(18)
stop=threading.Event()
thread=None

def run(command, **kw):
    result=subprocess.run(command, capture_output=True, text=True, **kw)
    if result.returncode:
        raise RuntimeError(command[0]+' exited '+str(result.returncode)+': '+result.stderr.replace(password,'[redacted]'))
    return result.stdout.strip()

def containers_snapshot():
    ids=run(['docker','ps','-q']).split()
    if not ids:return {}
    items=json.loads(run(['docker','inspect',*ids]))
    return {x['Id']:{'name':x['Name'],'started':x['State']['StartedAt']} for x in items}

sampler_command='''
for p in /proc/[0-9]*; do
  pid=${p##*/}
  comm=$(cat "$p/comm" 2>/dev/null) || continue
  if [ "$pid" = 1 ] || [ "$comm" = ffmpeg ]; then
    ticks=$(awk '{print $14+$15}' "$p/stat" 2>/dev/null)
    fds=$(ls "$p/fd" 2>/dev/null | wc -l)
    io=$(awk '{printf "%s=%s,",$1,$2}' "$p/io" 2>/dev/null)
    rss=$(awk '/VmRSS:/{print $2}' "$p/status" 2>/dev/null)
    printf 'PROC %s %s %s %s %s %s\\n' "$pid" "$comm" "$ticks" "$fds" "$rss" "$io"
  fi
done
bytes=$(find /tmp/p1-work -type f -exec stat -c '%s' {} \\; 2>/dev/null | awk '{s+=$1}END{print s+0}')
files=$(find /tmp/p1-work -type f 2>/dev/null | wc -l)
printf 'TEMP %s %s\\n' "$bytes" "$files"
'''

def sampler(name, stop, rows):
    started=time.monotonic()
    while not stop.is_set():
      try:
        result=subprocess.run(['docker','exec',name,'sh','-c',sampler_command],capture_output=True,text=True,timeout=3)
        if result.returncode==0:rows.append({'t':round(time.monotonic()-started,4),'sample':result.stdout})
      except subprocess.TimeoutExpired:
        rows.append({'t':round(time.monotonic()-started,4),'probe_timeout':True})
      stop.wait(.3)

for image_name in ['postgres:17-alpine','redis:7-alpine','go-recorder-minio:local','rabbitmq:4.2.9-management','go-recorder-worker']:
    run(['docker','image','inspect',image_name])
worker=json.loads(run(['docker','image','inspect','go-recorder-worker']))[0]
if worker['Os']!='linux' or worker['Architecture']!='arm64':
    raise RuntimeError('this harness requires the cached Linux arm64 worker image')
compile_env=dict(os.environ,GOOS='linux',GOARCH='arm64',CGO_ENABLED='0')
overlay=[]
capture_path=root/'internal/infrastructure/composite/capture.go'
selected_source=capture_path.read_bytes()
if phase=='before':
    selected_source=subprocess.run(['git','show',args.baseline_ref+':internal/infrastructure/composite/capture.go'],cwd=root,capture_output=True,check=True).stdout
    original=base/'capture-before.go.txt'; original.write_bytes(selected_source)
    overlay_path=base/'before-overlay.json'
    overlay_path.write_text(json.dumps({'Replace':{str(capture_path):str(original)}},indent=2))
    overlay=['-overlay',str(overlay_path)]
run(['go','test',*overlay,'-c','-o',str(base/'integration.test'),'./tests/integration'],cwd=root,env=compile_env)
if args.rabbit:
    run(['go','test','-c','-o',str(base/'rabbitmq.test'),'./internal/infrastructure/rabbitmq'],cwd=root,env=compile_env)
(evidence/'source-manifest.json').write_text(json.dumps({'phase':phase,'baseline_ref':args.baseline_ref if phase=='before' else None,'repository_head':run(['git','rev-parse','HEAD'],cwd=root),'capture_sha256':hashlib.sha256(selected_source).hexdigest(),'binary_sha256':hashlib.sha256((base/'integration.test').read_bytes()).hexdigest(),'worker_image':worker['Id']},indent=2))
print('evidence:',base,flush=True)
before=containers_snapshot()
(evidence/'containers-before.json').write_text(json.dumps(before,indent=2))
(evidence/'host-before.txt').write_text(run(['sh','-c','date; uptime; go version; docker version --format "{{.Server.Version}} {{.Server.Os}}/{{.Server.Arch}}"; docker stats --no-stream --format "{{.Name}} CPU={{.CPUPerc}} MEM={{.MemUsage}}"']))
status=1
try:
    net_created=True
    run(['docker','network','create','--internal','--label',label,network])
    created.append(names['pod'])
    run(['docker','run','-d','--name',names['pod'],'--label',label,'--network',network,'redis:7-alpine','redis-server','--save','','--appendonly','no'])
    shared=['--network','container:'+names['pod'],'--label',label]
    created.append(names['postgres'])
    run(['docker','run','-d','--name',names['postgres'],*shared,'-e','POSTGRES_USER=p1audit','-e','POSTGRES_PASSWORD='+password,'-e','POSTGRES_DB=p1audit','postgres:17-alpine','-c','listen_addresses=127.0.0.1'])
    created.append(names['minio'])
    run(['docker','run','-d','--name',names['minio'],*shared,'-e','MINIO_ROOT_USER=p1audit','-e','MINIO_ROOT_PASSWORD='+password,'go-recorder-minio:local','server','/data','--address','127.0.0.1:9000','--console-address','127.0.0.1:9001'])
    created.append(names['rabbit'])
    run(['docker','run','-d','--name',names['rabbit'],*shared,'-e','RABBITMQ_DEFAULT_USER=p1audit','-e','RABBITMQ_DEFAULT_PASS='+password,'rabbitmq:4.2.9-management'])
    probes={
      'postgres':['docker','exec',names['postgres'],'pg_isready','-U','p1audit','-d','p1audit'],
      'redis':['docker','exec',names['pod'],'redis-cli','ping'],
      'minio':['docker','exec',names['pod'],'wget','-q','-O','/dev/null','http://127.0.0.1:9000/minio/health/live'],
      'rabbit':['docker','exec','--user','rabbitmq',names['rabbit'],'rabbitmq-diagnostics','-q','ping'],
    }
    limit=time.monotonic()+75
    while probes and time.monotonic()<limit:
      for name,cmd in list(probes.items()):
        try:
          result=subprocess.run(cmd,capture_output=True,text=True,timeout=4)
          if result.returncode==0:del probes[name];print('ready:',name,flush=True)
        except subprocess.TimeoutExpired:pass
      if probes:time.sleep(1)
    if probes:raise RuntimeError('isolated service readiness failed: '+','.join(probes))
    env={
      'RECORDER_STAGE4_RECORDING_E2E':'true',
      'RECORDER_RECORDING_LOAD':'true',
      'RECORDER_P1_RECORDING_PROFILE':'true',
      'RECORDER_P1_PROFILE_DIR':'/evidence',
      'RECORDER_TEST_FFMPEG':'/usr/bin/ffmpeg',
      'RECORDER_STAGE1_TEST_POSTGRES_DSN':'postgres://p1audit:'+password+'@127.0.0.1:5432/p1audit?sslmode=disable',
      'RECORDER_STAGE2_TEST_REDIS_ADDR':'127.0.0.1:6379',
      'RECORDER_STAGE4_TEST_MINIO_ENDPOINT':'127.0.0.1:9000',
      'RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY':'p1audit',
      'RECORDER_STAGE4_TEST_MINIO_SECRET_KEY':password,
      'RECORDER_STAGE4_TEST_RABBIT_URL':'amqp://p1audit:'+password+'@127.0.0.1:5672/%2F',
      'RECORDER_STAGE4_ARTIFACT_DIR':'/evidence/stage4-artifacts',
      'TMPDIR':'/tmp/p1-work',
    }
    cmd=['docker','run','-d','--name',names['test'],*shared,'--workdir','/workspace/tests/integration']
    for key,value in env.items():cmd+=['-e',key+'='+value]
    for source,dest,readonly in [(base/'integration.test','/audit/integration.test',True),(root/'database/migrations','/workspace/database/migrations',True),(evidence,'/evidence',False)]:
      cmd+=['--mount','type=bind,source='+str(source)+',target='+dest+(',readonly' if readonly else '')]
    cmd+=['--entrypoint','sh','go-recorder-worker','-c','mkdir -p /tmp/p1-work; exec /audit/integration.test -test.run=^TestP1ConcurrentRecordingProfiles$ -test.cpuprofile=/evidence/recording-cpu.pprof -test.v -test.timeout=180s']
    created.append(names['test'])
    run(cmd)
    rows=[]
    thread=threading.Thread(target=sampler,args=(names['test'],stop,rows));thread.start()
    print('running concurrent full-stack pipelines with profiles and /proc sampling',flush=True)
    result=run(['docker','wait',names['test']],timeout=195)
    stop.set();thread.join()
    status=int(result)
    log=run(['docker','logs',names['test']])
    (evidence/'concurrent-full-stack.txt').write_text(log)
    checks={'decoded_content_rooms':log.count('decoded content validated:'),'recovered_closed_segment_rooms':log.count('expired recorder recovery finalized closed segment:'),'conference_auto_stop_rooms':log.count('conference finish auto-stop validated')}
    (evidence/'full-stack-checks.json').write_text(json.dumps(checks,indent=2))
    if status==0 and any(value!=2 for value in checks.values()):
      print('full-stack validation missing; do not treat skipped/empty workloads as success',flush=True)
      status=1
    (evidence/'process-samples.json').write_text(json.dumps(rows,indent=2))
    print('recording exit:',status,'samples:',len(rows),flush=True)
    rabbit_env='amqp://p1audit:'+password+'@127.0.0.1:5672/%2F'
    if not args.rabbit:
      rabbit_status=0
    else:
      rabbit_cmd=['docker','run','--name',names['rabbit-test'],*shared,'-e','RABBITMQ_TEST_URL='+rabbit_env,'--mount','type=bind,source='+str(base/'rabbitmq.test')+',target=/audit/rabbitmq.test,readonly','--mount','type=bind,source='+str(rabbit_evidence)+',target=/evidence','--entrypoint','/audit/rabbitmq.test','go-recorder-worker','-test.run=^TestConsumerReconnectRecoveryLatency$','-test.bench=^Benchmark(ConfirmedCommandSerial|ConfirmedCommandParallel|PublisherReconnect)$','-test.benchmem','-test.benchtime=1s','-test.count=3','-test.cpuprofile=/evidence/rabbitmq-cpu.pprof','-test.memprofile=/evidence/rabbitmq-allocs.pprof','-test.blockprofile=/evidence/rabbitmq-block.pprof','-test.mutexprofile=/evidence/rabbitmq-mutex.pprof','-test.timeout=120s','-test.v']
      created.append(names['rabbit-test'])
      with (rabbit_evidence/'confirmed-command-baseline.txt').open('w') as log:
        rabbit_status=subprocess.run(rabbit_cmd,stdout=log,stderr=subprocess.STDOUT,timeout=135).returncode
      print('rabbit exit:',rabbit_status,flush=True)
      status=status or rabbit_status
    status=status or rabbit_status
    (evidence/'setup.json').write_text(json.dumps({'network':network,'internal':True,'containers':names,'host_ports':[],'recording_exit':int(result),'rabbit_exit':rabbit_status,'git_head':run(['git','rev-parse','HEAD'],cwd=root),'worker_image':run(['docker','image','inspect','go-recorder-worker','--format','{{.Id}}'])},indent=2))
except Exception as error:
    print(type(error).__name__+': '+str(error).replace(password,'[redacted]'),flush=True)
finally:
    stop.set()
    if thread is not None: thread.join(timeout=5)
    removed=[]
    for name in reversed(created):
      inspected=subprocess.run(['docker','inspect',name],capture_output=True,text=True)
      if inspected.returncode==0:
        item=json.loads(inspected.stdout)[0]
        if item['Config'].get('Labels',{}).get(label_key)==tag:
          own_volumes=[m['Name'] for m in item.get('Mounts',[]) if m['Type']=='volume']
          volumes.extend(own_volumes)
          result=subprocess.run(['docker','rm','-f','-v',name],capture_output=True,text=True)
          removed.append({'name':name,'volumes':own_volumes,'remove_exit':result.returncode})
    if net_created:
      inspected=subprocess.run(['docker','network','inspect',network],capture_output=True,text=True)
      if inspected.returncode==0 and json.loads(inspected.stdout)[0].get('Labels',{}).get(label_key)==tag:
        subprocess.run(['docker','network','rm',network],capture_output=True,text=True)
    after=containers_snapshot()
    (evidence/'containers-after.json').write_text(json.dumps(after,indent=2))
    unchanged=all(after.get(key)==value for key,value in before.items())
    leftovers=run(['docker','ps','-aq','--filter','label='+label]).split()
    net_leftovers=run(['docker','network','ls','-q','--filter','label='+label]).split()
    volume_leftovers=[v for v in volumes if subprocess.run(['docker','volume','inspect',v],capture_output=True,text=True).returncode==0]
    (evidence/'cleanup.json').write_text(json.dumps({'existing_containers_unchanged':unchanged,'created_container_leftovers':leftovers,'created_network_leftovers':net_leftovers,'created_volume_leftovers':volume_leftovers,'removed':removed},indent=2))
    print('cleanup: existing unchanged=',unchanged,' container/network/volume leftovers=',len(leftovers),len(net_leftovers),len(volume_leftovers),flush=True)
    if leftovers or net_leftovers or volume_leftovers or any(item['remove_exit'] for item in removed):status=1
raise SystemExit(status)
