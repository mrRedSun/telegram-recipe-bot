#!/usr/bin/env python3
"""Run explicit developer-authored retrieval probes; never import user requests or recipe output."""
import argparse,importlib.util,json,sqlite3,statistics,time
from pathlib import Path

QUERIES=[
 ("chicken tikka masala",["chicken","tomato","yogurt"]),
 ("chicken kiev",["chicken","garlic","parsley"]),
 ("borscht",["beet","cabbage","potato"]),
 ("cottage cheese pancakes",["cottage cheese"]),
 ("chocolate brownies",["chocolate","cocoa"]),
 ("pasta carbonara",["pasta","bacon","parmesan"]),
 ("lasagna",["beef","tomato","mozzarella"]),
 ("hummus",["chickpea","tahini","lemon"]),
 ("falafel",["chickpea","cumin","parsley"]),
 ("creme brulee",["cream","vanilla"]),
]

def main():
 parser=argparse.ArgumentParser();parser.add_argument("database");args=parser.parse_args()
 module=Path(__file__).resolve().parents[2]/"internal/catalog/search.py"
 spec=importlib.util.spec_from_file_location("catalog_search",module);helper=importlib.util.module_from_spec(spec);spec.loader.exec_module(helper)
 db=sqlite3.connect(Path(args.database).resolve().as_uri()+"?mode=ro&immutable=1",uri=True)
 db.execute("PRAGMA query_only=ON");db.execute("PRAGMA cache_size=-8192")
 rows=[]
 for dish,ingredients in QUERIES:
  for iteration in range(3):
   start=time.monotonic();matches=helper.search(db,{"dish":dish,"ingredients":ingredients});elapsed=(time.monotonic()-start)*1000
   rows.append({"query":dish,"iteration":iteration,"matches":len(matches),"top_title":matches[0]["title"] if matches else None,"top_score":matches[0]["score"] if matches else None,"elapsed_ms":round(elapsed,2)})
 timings=sorted(row["elapsed_ms"] for row in rows)
 # Relevant title/ingredient matches are an availability check, not a correctness benchmark.
 print(json.dumps({"probe_count":len(rows),"queries_with_matches":sum(row["matches"]>0 for row in rows[::3]),"median_ms":statistics.median(timings),"p95_ms":timings[int(len(timings)*.95)-1],"max_ms":max(timings),"probes":rows},indent=2))

if __name__=="__main__":main()
