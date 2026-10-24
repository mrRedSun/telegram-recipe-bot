#!/usr/bin/env python3
"""Build an immutable recipe catalogue from Food.com CSV or normalized JSONL; stdlib only."""
import argparse,csv,hashlib,html,json,os,re,sqlite3,sys,time,unicodedata
from pathlib import Path

DATASET="Food.com recipes (Kaggle irkaal, 2020-12-28)"
LICENSE="CC0-1.0"

def text(value):
    if not isinstance(value,str):return ""
    return " ".join(html.unescape(value).split())

def r_list(raw):
    raw=raw.strip()
    if raw in ("NA","NULL","character(0)",""):return []
    if not (raw.startswith("c(") and raw.endswith(")")):
        value=json.loads(raw)
        if not isinstance(value,str):raise ValueError()
        return [value]
    # Decode only quoted strings and literal NA, without evaluating R or Python code.
    remaining=raw[2:-1].strip();out=[];decoder=json.JSONDecoder()
    while remaining:
        if remaining.startswith("NA") and (len(remaining)==2 or remaining[2] in ", \t\n"):
            value="";remaining=remaining[2:]
        else:
            value,end=decoder.raw_decode(remaining);remaining=remaining[end:]
            if not isinstance(value,str):raise ValueError()
        out.append(value);remaining=remaining.strip()
        if not remaining:break
        if not remaining.startswith(","):raise ValueError()
        remaining=remaining[1:].strip()
        if not remaining:raise ValueError()
    return out

def food_rows(source):
    for row in csv.DictReader(source):
        try:
            parts=r_list(row["RecipeIngredientParts"])
            # Upstream quantity arrays frequently have a different length and omit units.
            # Keep ingredient names; never attach unpaired or unitless numeric quantities.
            ingredients=[text(part) for part in parts]
            title=text(row["Name"]);identifier=row["RecipeId"]
            if not identifier.isdigit():raise ValueError()
            slug=re.sub(r"[^a-z0-9]+","-",unicodedata.normalize("NFKD",title).encode("ascii","ignore").decode().lower()).strip("-")
            yield {"title":title,"ingredients":ingredients,"steps":r_list(row["RecipeInstructions"]),"url":f"https://www.food.com/recipe/{slug}-{identifier}"}
        except (ValueError,KeyError,TypeError):yield None

def normalize(row):
    from urllib.parse import urlsplit
    if not isinstance(row,dict):return None
    title=text(row.get("title")); ingredients=row.get("ingredients");steps=row.get("steps");url=row.get("url","")
    if not title or len(title)>240 or not isinstance(ingredients,list) or not isinstance(steps,list):return None
    ingredients=[text(x) for x in ingredients];steps=[text(x) for x in steps]
    if not 1<=len(ingredients)<=40 or not 1<=len(steps)<=30:return None
    if not all(0<len(x)<=240 for x in ingredients) or not all(0<len(x)<=1000 for x in steps):return None
    u=urlsplit(url)
    if u.scheme!="https" or not u.hostname or u.username or u.password or len(url)>1200:return None
    return title,ingredients,steps,url

def build(args):
    started=time.monotonic()
    if args.format=="jsonl" and (not args.dataset or not args.license):
        raise ValueError("normalized imports require explicit provenance")
    args.dataset=args.dataset or DATASET
    args.license=args.license or LICENSE
    if args.limit<0:raise ValueError("limit must be nonnegative")
    output=Path(args.output)
    # Exclusive creation protects existing catalogues, including a failed prior import.
    fd=os.open(output,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600);os.close(fd)
    db=sqlite3.connect(output)
    db.executescript("""PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL; PRAGMA temp_store=FILE;
    CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
    CREATE TABLE recipes(id INTEGER PRIMARY KEY,title TEXT NOT NULL,ingredients TEXT NOT NULL,
      steps TEXT NOT NULL,url TEXT NOT NULL,dataset TEXT NOT NULL,license TEXT NOT NULL,fingerprint TEXT UNIQUE NOT NULL);
    CREATE VIRTUAL TABLE recipe_fts USING fts5(title,ingredients,content='recipes',content_rowid='id',tokenize='porter unicode61');
    """)
    counters={"read":0,"imported":0,"invalid":0,"duplicates":0}
    with open(args.input,encoding="utf-8-sig",newline="") as source:
        csv.field_size_limit(2<<20)
        rows=food_rows(source) if args.format=="foodcom" else (json.loads(line) for line in source if line.strip())
        for raw in rows:
            counters["read"]+=1
            row=normalize(raw)
            if row is None:counters["invalid"]+=1;continue
            title,ingredients,steps,url=row
            ing=json.dumps(ingredients,ensure_ascii=False);step=json.dumps(steps,ensure_ascii=False)
            fingerprint=hashlib.sha256((title.casefold()+"\n"+ing.casefold()+"\n"+step.casefold()).encode()).hexdigest()
            before=db.total_changes
            db.execute("INSERT OR IGNORE INTO recipes(title,ingredients,steps,url,dataset,license,fingerprint) VALUES(?,?,?,?,?,?,?)",(title,ing,step,url,args.dataset,args.license,fingerprint))
            if db.total_changes==before:counters["duplicates"]+=1
            else:counters["imported"]+=1
            if counters["read"]%10000==0:db.commit()
            if args.limit and counters["imported"]>=args.limit:break
    if not counters["imported"]:raise ValueError("no valid recipes")
    db.execute("INSERT INTO recipe_fts(recipe_fts) VALUES('rebuild')")
    db.execute("INSERT INTO recipe_fts(recipe_fts) VALUES('optimize')")
    with open(args.input,"rb") as source_bytes:
        digest=hashlib.file_digest(source_bytes,"sha256").hexdigest()
    metadata={"schema_version":"1","records":str(counters["imported"]),"dataset":args.dataset,"license":args.license,"input_sha256":digest,"status":"ready"}
    db.commit()
    if db.execute("PRAGMA integrity_check").fetchone()!=("ok",):raise ValueError("integrity check failed")
    db.executemany("INSERT INTO metadata VALUES(?,?)",metadata.items());db.commit()
    db.close();output.chmod(0o444)
    result={**counters,"input_sha256":digest,"database_bytes":output.stat().st_size,"elapsed_seconds":round(time.monotonic()-started,2)}
    print(json.dumps(result))

if __name__=="__main__":
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input",required=True);parser.add_argument("--output",required=True)
    parser.add_argument("--format",choices=["foodcom","jsonl"],default="foodcom")
    parser.add_argument("--dataset");parser.add_argument("--license")
    parser.add_argument("--limit",type=int,default=0)
    try:build(parser.parse_args())
    except Exception:
        print("Catalogue import failed; existing outputs were not overwritten. Incomplete output is retained for diagnosis.",file=sys.stderr)
        sys.exit(1)
