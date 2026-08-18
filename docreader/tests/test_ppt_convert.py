import shutil
import unittest
from pathlib import Path

from docreader.parser.ppt_convert import (
    convert_ppt_to_pptx_bytes,
    is_ole_compound,
    is_zip_openxml,
    needs_ppt_to_pptx_conversion,
    normalize_ppt_bytes,
)

TESTDATA = Path(__file__).resolve().parents[2] / "testdata" / "rag_test"
LEGACY_PPT = TESTDATA / "ppt_old" / "en_38256.ppt"
WMF_IMAGE_PPT = LEGACY_PPT
IMAGE_HEAVY_PPT = TESTDATA / "ppt_old" / "en_41384.ppt"
PPTX_SAMPLE = TESTDATA / "pptx" / "en_marker.pptx"
OLE_SAMPLE = b"\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1" + (b"\x00" * 32)
PPTX_SAMPLE_BYTES = b"PK\x03\x04" + (b"\x00" * 32)


class TestPptConvert(unittest.TestCase):
    def test_legacy_ppt_magic(self):
        content = OLE_SAMPLE
        self.assertTrue(is_ole_compound(content))
        self.assertFalse(is_zip_openxml(content))
        self.assertTrue(needs_ppt_to_pptx_conversion(content, "ppt"))

    def test_pptx_does_not_need_conversion(self):
        content = PPTX_SAMPLE_BYTES
        self.assertTrue(is_zip_openxml(content))
        self.assertFalse(needs_ppt_to_pptx_conversion(content, "pptx"))

    def test_normalize_pptx_passthrough(self):
        content = PPTX_SAMPLE_BYTES
        out, ext = normalize_ppt_bytes(content, "pptx")
        self.assertEqual(out, content)
        self.assertEqual(ext, ".pptx")

    def test_legacy_ppt_requires_soffice(self):
        if not shutil.which("soffice"):
            with self.assertRaises(ValueError) as ctx:
                normalize_ppt_bytes(OLE_SAMPLE, "ppt")
            self.assertIn("LibreOffice", str(ctx.exception))
            self.skipTest("LibreOffice not available")
        if not LEGACY_PPT.is_file():
            self.skipTest("legacy PPT testdata missing")
        converted = convert_ppt_to_pptx_bytes(LEGACY_PPT.read_bytes(), suffix=".ppt")
        self.assertIsNotNone(converted)
        self.assertTrue(is_zip_openxml(converted))
        out, ext = normalize_ppt_bytes(LEGACY_PPT.read_bytes(), "ppt")
        self.assertEqual(ext, ".pptx")
        self.assertTrue(is_zip_openxml(out))

    def test_wmf_legacy_ppt_extracts_rasterized_image(self):
        if not shutil.which("soffice"):
            self.skipTest("LibreOffice not available")
        if not shutil.which("convert"):
            self.skipTest("ImageMagick convert not available")
        if not WMF_IMAGE_PPT.is_file():
            self.skipTest("testdata missing")

        from docreader.parser.markitdown_parser import MarkitdownParser

        doc = MarkitdownParser(file_type="ppt").parse_into_text(
            WMF_IMAGE_PPT.read_bytes()
        )
        self.assertEqual(len(doc.images), 1)
        self.assertNotIn("bd10496_.jpg", doc.content)
        self.assertIn("images/", doc.content)

    def test_image_heavy_legacy_ppt_extracts_images(self):
        if not shutil.which("soffice"):
            self.skipTest("LibreOffice not available")
        if not IMAGE_HEAVY_PPT.is_file():
            self.skipTest("testdata missing")

        from docreader.parser.markitdown_parser import MarkitdownParser

        doc = MarkitdownParser(file_type="ppt").parse_into_text(
            IMAGE_HEAVY_PPT.read_bytes()
        )
        self.assertGreaterEqual(len(doc.images), 2)
        self.assertNotIn("![](.jpg)", doc.content)
        for ref in doc.images:
            self.assertTrue(ref.startswith("images/"))


if __name__ == "__main__":
    unittest.main()
