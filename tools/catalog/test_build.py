import argparse,contextlib,io,json,sqlite3,tempfile,unittest
from pathlib import Path
import build

class ImportTests(unittest.TestCase):
    def test_r_vectors_are_data_not_code(self):
        self.assertEqual(build.r_list('c("1/2", "2")'),["1/2","2"])
        self.assertEqual(build.r_list('"salt"'),["salt"])
        self.assertEqual(build.r_list('character(0)'),[])
        self.assertEqual(build.r_list('c(NA, "2", "NA")'),["","2","NA"])
        with self.assertRaises(ValueError):build.r_list('__import__("os").system("echo BAD")')

    def test_filter_deduplicate_finalize_and_protect_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);source=root/"recipes.jsonl";output=root/"catalog.sqlite"
            recipe={"title":"Chicken soup","ingredients":["1 cup chicken"],"steps":["Simmer."],"url":"https://example.com/chicken"}
            bad=dict(recipe,url="javascript:alert(1)")
            source.write_text("\n".join(map(json.dumps,[recipe,recipe,bad])))
            args=argparse.Namespace(input=str(source),output=str(output),format="jsonl",dataset="Test fixtures",license="CC0-1.0",limit=0)
            with contextlib.redirect_stdout(io.StringIO()):build.build(args)
            db=sqlite3.connect(output)
            self.assertEqual(db.execute("SELECT count(*) FROM recipes").fetchone()[0],1)
            self.assertEqual(dict(db.execute("SELECT key,value FROM metadata"))["status"],"ready")
            self.assertEqual(db.execute("SELECT count(*) FROM recipe_fts WHERE recipe_fts MATCH 'chicken'").fetchone()[0],1)
            db.close();before=output.read_bytes()
            with self.assertRaises(FileExistsError):build.build(args)
            self.assertEqual(output.read_bytes(),before)

    def test_foodcom_never_pairs_unaligned_or_unitless_quantities(self):
        source=io.StringIO('RecipeId,Name,RecipeIngredientParts,RecipeIngredientQuantities,RecipeInstructions\n1,Soup,"c(""chicken"", ""carrot"")","c(""999"", NA, ""12"")","""Simmer."""\n')
        row=next(build.food_rows(source))
        self.assertEqual(row["ingredients"],["chicken","carrot"])

    def test_normalized_source_requires_explicit_provenance(self):
        args=argparse.Namespace(format="jsonl",dataset=None,license=None)
        with self.assertRaises(ValueError):build.build(args)

    def test_reject_malformed_or_excessive_recipes(self):
        self.assertIsNone(build.normalize({"title":"Test","ingredients":["x"]*41,"steps":["mix"],"url":"https://example.com"}))
        self.assertIsNone(build.normalize({"title":"Test","ingredients":["x"],"steps":["mix"],"url":"https://user:secret@example.com"}))

if __name__=="__main__":unittest.main()
