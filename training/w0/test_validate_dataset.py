import json
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import validate_dataset as validator  # noqa: E402


def record(identifier: str, text: str, **changes) -> dict:
    value = {
        "id": identifier,
        "text": text,
        "source": "fixture",
        "source_type": "documentation",
        "language": "en",
        "license": "fixture-only",
        "split": "train",
        "synthetic": False,
        "content_sha256": validator.content_sha256(text),
    }
    value.update(changes)
    return value


class DatasetValidatorTest(unittest.TestCase):
    def run_validation(
        self,
        values: list[dict],
        denied: set[str] | None = None,
        benchmark_values: list[dict] | None = None,
        **options,
    ) -> dict:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            shard = root / "part.jsonl"
            shard.write_text("".join(json.dumps(value) + "\n" for value in values), encoding="utf-8")
            if benchmark_values is not None:
                benchmark_shard = root / "benchmark.jsonl"
                benchmark_shard.write_text(
                    "".join(json.dumps(value) + "\n" for value in benchmark_values), encoding="utf-8"
                )
                options["benchmark_files"] = [benchmark_shard]
            return validator.validate(
                [shard],
                min_chars=1,
                max_bytes=4096,
                deny_hashes=denied or set(),
                index_path=root / "index.sqlite3",
                max_errors=20,
                **options,
            )

    def test_valid_real_and_synthetic_records(self):
        synthetic = record(
            "synth-1",
            "A checked synthetic textbook passage.",
            synthetic=True,
            generator="generator/model@revision",
            generation_prompt_hash="a" * 64,
        )
        report = self.run_validation([record("real-1", "A licensed real passage."), synthetic])
        self.assertTrue(report["valid"])
        self.assertEqual(report["records"], 2)
        self.assertEqual(report["accepted_records"], 2)
        self.assertEqual(report["synthetic_records"], 1)

    def test_duplicate_id_and_content_are_rejected(self):
        text = "The same document appears twice."
        report = self.run_validation([record("duplicate", text), record("duplicate", text)])
        self.assertFalse(report["valid"])
        self.assertEqual(report["duplicate_ids"], 1)
        self.assertEqual(report["duplicate_contents"], 1)
        self.assertEqual(report["accepted_records"], 1)

    def test_hash_mismatch_and_missing_synthetic_provenance_are_rejected(self):
        value = record("synth", "Generated material.", synthetic=True, content_sha256="b" * 64)
        report = self.run_validation([value])
        self.assertFalse(report["valid"])
        self.assertIn("does not match", report["errors"][0]["error"])
        self.assertIn("no generator", report["errors"][0]["error"])

    def test_denied_evaluation_content_is_rejected(self):
        value = record("contaminated", "Reserved evaluation example.")
        report = self.run_validation([value], {value["content_sha256"]})
        self.assertFalse(report["valid"])
        self.assertEqual(report["denied_contents"], 1)
        self.assertEqual(report["accepted_records"], 0)

    def test_canonical_normalization_is_required(self):
        decomposed = "Cafe\u0301"
        report = self.run_validation([record("unicode", decomposed)])
        self.assertFalse(report["valid"])
        self.assertIn("not canonical", report["errors"][0]["error"])

    def test_pii_scan_rejects_email_and_private_key_by_default(self):
        with_email = record("email-leak", "Contact the maintainer at leak@example.com for details.")
        with_key = record(
            "key-leak",
            "-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAK...\n-----END RSA PRIVATE KEY-----",
        )
        report = self.run_validation([with_email, with_key])
        self.assertFalse(report["valid"])
        self.assertEqual(report["pii_records"], 2)
        self.assertEqual(report["accepted_records"], 0)

    def test_pii_scan_can_be_disabled(self):
        with_email = record("email-allowed", "Contact the maintainer at leak@example.com for details.")
        report = self.run_validation([with_email], pii_scan=False)
        self.assertTrue(report["valid"])
        self.assertEqual(report["pii_records"], 0)
        self.assertEqual(report["accepted_records"], 1)

    def test_clean_text_passes_pii_scan(self):
        report = self.run_validation([record("clean", "A perfectly ordinary licensed passage of prose.")])
        self.assertTrue(report["valid"])
        self.assertEqual(report["pii_records"], 0)

    def test_license_allowlist_rejects_unapproved_license(self):
        approved = record("approved", "Text under an approved license.", license="cc-by-4.0")
        rejected = record("rejected", "Text under an unapproved license.", license="all-rights-reserved")
        report = self.run_validation(
            [approved, rejected], license_allow={"cc-by-4.0", "fixture-only"}
        )
        self.assertFalse(report["valid"])
        self.assertEqual(report["license_rejected"], 1)
        self.assertEqual(report["accepted_records"], 1)
        self.assertEqual(report["by_license"], {"cc-by-4.0": 1})

    NEAR_DUP_A = (
        "The quick brown fox jumps over the lazy dog near the river bank while birds sing "
        "softly in the tall green trees under a clear blue morning sky."
    )
    NEAR_DUP_B = (
        "The quick brown fox jumps over the lazy dog beside the river bank while birds sing "
        "softly in the tall green trees under a clear blue morning sky."
    )
    DISSIMILAR = (
        "Quarterly revenue grew twelve percent on strength in cloud services, offsetting a "
        "slower quarter for the hardware division across every major region."
    )

    def test_near_duplicate_text_is_rejected(self):
        original = record("original", self.NEAR_DUP_A)
        near_copy = record("near-copy", self.NEAR_DUP_B)
        report = self.run_validation([original, near_copy], near_duplicate=True)
        self.assertFalse(report["valid"])
        self.assertEqual(report["near_duplicate_records"], 1)
        self.assertEqual(report["accepted_records"], 1)

    def test_near_duplicate_disabled_by_default(self):
        original = record("original", self.NEAR_DUP_A)
        near_copy = record("near-copy", self.NEAR_DUP_B)
        report = self.run_validation([original, near_copy])
        self.assertTrue(report["valid"])
        self.assertEqual(report["near_duplicate_records"], 0)
        self.assertEqual(report["accepted_records"], 2)

    def test_dissimilar_texts_are_not_flagged_as_near_duplicates(self):
        first = record("first", self.NEAR_DUP_A)
        second = record("second", self.DISSIMILAR)
        report = self.run_validation([first, second], near_duplicate=True)
        self.assertTrue(report["valid"])
        self.assertEqual(report["near_duplicate_records"], 0)

    def test_near_duplicate_requires_bands_to_divide_minhash_count(self):
        with self.assertRaises(RuntimeError):
            self.run_validation(
                [record("only", "Any text works for this configuration error check.")],
                near_duplicate=True,
                num_minhashes=10,
                lsh_bands=3,
            )

    def test_benchmark_contamination_is_rejected_and_distinguished_from_near_duplicate(self):
        benchmark = [record("benchmark-1", self.NEAR_DUP_A)]
        target = record("target-1", self.NEAR_DUP_B)
        report = self.run_validation([target], benchmark_values=benchmark, near_duplicate=True)
        self.assertFalse(report["valid"])
        self.assertEqual(report["benchmark_contaminated_records"], 1)
        self.assertEqual(report["near_duplicate_records"], 0)
        self.assertEqual(report["accepted_records"], 0)
        self.assertEqual(report["benchmark_records_seeded"], 1)

    def test_benchmark_corpus_requires_near_duplicate_enabled(self):
        with self.assertRaises(RuntimeError):
            self.run_validation(
                [record("target-1", self.NEAR_DUP_B)],
                benchmark_values=[record("benchmark-1", self.NEAR_DUP_A)],
            )

    def test_dissimilar_text_is_not_flagged_as_benchmark_contamination(self):
        benchmark = [record("benchmark-1", self.NEAR_DUP_A)]
        target = record("target-1", self.DISSIMILAR)
        report = self.run_validation([target], benchmark_values=benchmark, near_duplicate=True)
        self.assertTrue(report["valid"])
        self.assertEqual(report["benchmark_contaminated_records"], 0)

    def test_max_records_per_source_rejects_beyond_cap(self):
        values = [record(f"s{i}", f"Distinct passage number {i} about the topic.", source="feed-a") for i in range(3)]
        report = self.run_validation(values, max_records_per_source=2)
        self.assertFalse(report["valid"])
        self.assertEqual(report["accepted_records"], 2)
        self.assertEqual(report["source_cap_rejected"], 1)

    def test_max_records_per_source_type_rejects_beyond_cap(self):
        values = [
            record(f"t{i}", f"Distinct passage number {i} about the topic.", source=f"feed-{i}", source_type="forum")
            for i in range(3)
        ]
        report = self.run_validation(values, max_records_per_source_type=2)
        self.assertFalse(report["valid"])
        self.assertEqual(report["accepted_records"], 2)
        self.assertEqual(report["source_cap_rejected"], 1)

    def test_source_caps_disabled_by_default(self):
        values = [record(f"u{i}", f"Distinct passage number {i} about the topic.", source="feed-a") for i in range(3)]
        report = self.run_validation(values)
        self.assertTrue(report["valid"])
        self.assertEqual(report["accepted_records"], 3)

    def test_language_script_check_rejects_structural_mismatch(self):
        cyrillic_text = "Съешь ещё этих мягких французских булок да выпей чаю пожалуйста."
        report = self.run_validation([record("mismatch", cyrillic_text, language="en")], language_script_check=True)
        self.assertFalse(report["valid"])
        self.assertIn("expects latin script", report["errors"][0]["error"])

    def test_language_script_check_accepts_matching_script(self):
        report = self.run_validation(
            [record("match", "An ordinary English-language passage of prose.", language="en")],
            language_script_check=True,
        )
        self.assertTrue(report["valid"])

    def test_language_script_check_disabled_by_default(self):
        cyrillic_text = "Съешь ещё этих мягких французских булок да выпей чаю пожалуйста."
        report = self.run_validation([record("mismatch", cyrillic_text, language="en")])
        self.assertTrue(report["valid"])

    def test_max_token_repetition_ratio_rejects_degenerate_text(self):
        degenerate = " ".join(["spam"] * 20 + ["ham"] * 2)
        report = self.run_validation([record("degenerate", degenerate)], max_token_repetition_ratio=0.5)
        self.assertFalse(report["valid"])
        self.assertIn("single token exceeds", report["errors"][0]["error"])

    def test_min_alpha_ratio_rejects_symbol_heavy_text(self):
        symbol_heavy = "### 1234567890 ### %%%%%%%% :::::::: ++++++++ ========"
        report = self.run_validation([record("symbols", symbol_heavy)], min_alpha_ratio=0.5)
        self.assertFalse(report["valid"])
        self.assertIn("alphabetic character ratio", report["errors"][0]["error"])

    def test_quality_heuristics_disabled_by_default(self):
        degenerate = " ".join(["spam"] * 20 + ["ham"] * 2)
        report = self.run_validation([record("degenerate", degenerate)])
        self.assertTrue(report["valid"])


class DetectPiiTest(unittest.TestCase):
    def test_detects_email(self):
        self.assertEqual(validator.detect_pii("reach me at person@example.com please"), ["email"])

    def test_detects_aws_access_key(self):
        categories = validator.detect_pii("key=AKIAABCDEFGHIJKLMNOP in the config")
        self.assertIn("aws_access_key", categories)

    def test_detects_valid_credit_card_via_luhn(self):
        categories = validator.detect_pii("card number 4111 1111 1111 1111 on file")
        self.assertIn("credit_card", categories)

    def test_ignores_random_long_digit_sequences(self):
        self.assertEqual(validator.detect_pii("invoice total was 1234567890123456 units"), [])

    def test_ordinary_prose_has_no_matches(self):
        self.assertEqual(validator.detect_pii("The lazy dog slept in the warm afternoon sun."), [])


class ShingleAndMinhashTest(unittest.TestCase):
    def test_lsh_bucket_keys_collide_for_identical_signatures(self):
        signature = validator.minhash_signature({"a b c", "b c d"}, num_hashes=8)
        self.assertEqual(
            validator.lsh_bucket_keys(signature, bands=4),
            validator.lsh_bucket_keys(signature, bands=4),
        )

    def test_shingles_short_text_falls_back_to_whole_text(self):
        self.assertEqual(validator.shingles("hi there", size=5), {"hi there"})

    def test_shingles_empty_text_is_empty_set(self):
        self.assertEqual(validator.shingles("", size=3), set())


class LanguageScriptTest(unittest.TestCase):
    def test_dominant_script_detects_latin(self):
        self.assertEqual(validator.dominant_script("An ordinary English sentence."), "latin")

    def test_dominant_script_detects_cyrillic(self):
        self.assertEqual(validator.dominant_script("Съешь ещё этих мягких французских булок."), "cyrillic")

    def test_dominant_script_none_when_no_recognized_script_chars(self):
        self.assertIsNone(validator.dominant_script("1234 567 !!! ###"))

    def test_language_script_mismatch_flags_cross_script_text(self):
        mismatch = validator.language_script_mismatch("en", "Съешь ещё этих мягких французских булок.")
        self.assertIsNotNone(mismatch)
        self.assertIn("latin", mismatch)
        self.assertIn("cyrillic", mismatch)

    def test_language_script_mismatch_accepts_matching_text(self):
        self.assertIsNone(validator.language_script_mismatch("en", "An ordinary English sentence."))

    def test_language_script_mismatch_skips_unmapped_language(self):
        self.assertIsNone(validator.language_script_mismatch("eo", "Sxauma programo por gxia."))

    def test_language_script_mismatch_handles_region_suffix(self):
        self.assertIsNone(validator.language_script_mismatch("en-US", "An ordinary English sentence."))


class QualityHeuristicsTest(unittest.TestCase):
    def test_dominant_token_ratio_detects_repetition(self):
        text = " ".join(["spam"] * 9 + ["ham"])
        self.assertAlmostEqual(validator.dominant_token_ratio(text), 0.9)

    def test_dominant_token_ratio_empty_text_is_zero(self):
        self.assertEqual(validator.dominant_token_ratio(""), 0.0)

    def test_dominant_token_ratio_diverse_text_is_low(self):
        text = "every single word here is completely different from the rest"
        self.assertLess(validator.dominant_token_ratio(text), 0.2)

    def test_alpha_character_ratio_all_letters(self):
        self.assertEqual(validator.alpha_character_ratio("hello world"), 1.0)

    def test_alpha_character_ratio_symbol_heavy(self):
        self.assertLess(validator.alpha_character_ratio("### 123 %%% +++"), 0.2)

    def test_alpha_character_ratio_empty_text_is_zero(self):
        self.assertEqual(validator.alpha_character_ratio("   "), 0.0)


if __name__ == "__main__":
    unittest.main()
