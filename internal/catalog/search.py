"""Read-only, bounded SQLite FTS search. All requests arrive on stdin, never argv/logs."""
import json, re, sqlite3, sys, time, unicodedata
from pathlib import Path

STOP = set("a an and the of with in on for to or fresh dried chopped ground finely minced sliced whole cup cups tablespoon tablespoons teaspoon teaspoons salt pepper water oil butter sugar flour egg eggs how make easy best recipe homemade delicious simple tasty cooking video reel short".split())

def words(text):
    text = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode().lower()
    return [w for w in re.findall(r"[a-z]{3,30}", text) if w not in STOP]

def stems(text):
    return {w[:-1] if w.endswith("s") and len(w)>4 else w for w in words(text)}

def search(db, request):
    dish = str(request.get("dish", ""))[:160]
    ingredients = request.get("ingredients", []) or []
    if not isinstance(ingredients, list):
        raise ValueError()
    ingredients = [x[:80] for x in ingredients[:8] if isinstance(x,str)]
    title_terms = list(dict.fromkeys(words(dish)))[:8]
    ingredient_terms = list(dict.fromkeys(w for x in ingredients for w in words(x)))[:16]
    terms = list(dict.fromkeys(title_terms + ingredient_terms))
    if not terms:
        return []
    match = " OR ".join('"'+term+'"' for term in terms)
    rows = db.execute("""SELECT r.title,r.ingredients,r.steps,r.url,r.dataset,r.license,
        bm25(recipe_fts,8,1) FROM recipe_fts JOIN recipes r ON r.id=recipe_fts.rowid
        WHERE recipe_fts MATCH ? ORDER BY bm25(recipe_fts,8,1) LIMIT 100""", (match,)).fetchall()
    dish_words=stems(dish)
    ingredient_words=[stems(x) for x in ingredients if stems(x)]
    candidates=[]
    for title, ing_raw, steps_raw, url, dataset, license_name, rank in rows:
        ing=json.loads(ing_raw); steps=json.loads(steps_raw)
        title_words=stems(title); all_ing=stems(" ".join(ing))
        title_score=len(dish_words & title_words)/max(1,len(dish_words))
        overlaps=sum(len(x & all_ing)/len(x)>=0.6 for x in ingredient_words)
        ingredient_score=overlaps/max(1,len(ingredient_words))
        # Common pantry staples alone cannot qualify an unrelated recipe.
        qualifies=(bool(dish_words) and title_score>=0.6 and (not ingredient_words or ingredient_score>=0.4))
        qualifies=qualifies or (len(ingredient_words)>=3 and overlaps>=3 and ingredient_score>=0.75)
        if not qualifies:
            continue
        score=0.65*title_score+0.35*ingredient_score
        item={"title":title,"ingredients":ing[:35],"steps":steps[:25],"url":url,
              "dataset":dataset,"license":license_name,"score":round(score,4)}
        candidates.append((score,rank,item))
    candidates.sort(key=lambda x:(-x[0],x[1]))
    out=[]; budget=24000
    for _,_,item in candidates:
        size=len(json.dumps(item,ensure_ascii=False).encode())
        if size>budget:
            continue
        out.append(item);budget-=size
        if len(out)==3:
            break
    return out

def main():
    path=Path(sys.argv[1]).resolve()
    deadline=time.monotonic()+2.5
    db=sqlite3.connect(path.as_uri()+"?mode=ro&immutable=1",uri=True,timeout=1)
    db.execute("PRAGMA query_only=ON")
    db.execute("PRAGMA cache_size=-8192")
    db.set_progress_handler(lambda:int(time.monotonic()>deadline),2000)
    metadata=dict(db.execute("SELECT key,value FROM metadata"))
    if metadata.get("schema_version")!="1" or metadata.get("status")!="ready" or int(metadata.get("records","0"))<1:
        raise ValueError()
    if sys.argv[2]=="info":
        db.execute("SELECT rowid FROM recipe_fts LIMIT 1").fetchone()
        result={"records":int(metadata["records"]),"dataset":metadata["dataset"],"license":metadata["license"]}
    else:
        request=json.loads(sys.stdin.buffer.read(16385))
        result=search(db,request)
    print(json.dumps(result,ensure_ascii=False))

if __name__=="__main__":
    try:main()
    except Exception:
        # Neither source text, paths, queries, nor SQLite exception details leave the helper.
        sys.exit(1)
