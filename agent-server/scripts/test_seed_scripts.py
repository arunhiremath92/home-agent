import contextlib
import io
import json
import unittest
from unittest import mock
from urllib.error import HTTPError, URLError

import seed_location_info
import seed_task


class SeedLocationInfoTests(unittest.TestCase):
    def setUp(self):
        self.response = mock.MagicMock()
        self.response.__enter__.return_value = self.response
        self.response.status = 200
        self.response.read.return_value = b"ok"

    def run_script(self, arguments):
        with mock.patch(
            "seed_location_info.urllib.request.urlopen", return_value=self.response
        ) as urlopen:
            with contextlib.redirect_stdout(io.StringIO()):
                seed_location_info.main(arguments)
        return urlopen

    def test_sends_each_message_with_provided_coordinates(self):
        urlopen = self.run_script(
            [
                "--store-name",
                "Juniper & Crumb Cafe",
                "--store-type",
                "restaurant",
                "--message",
                "Usually order a latte.",
                "--message",
                "The croissants are great.",
                "--latitude",
                "40.7580",
                "--longitude",
                "-73.9855",
            ]
        )

        self.assertEqual(urlopen.call_count, 2)
        first_request = urlopen.call_args_list[0].args[0]
        first_payload = json.loads(first_request.data)
        self.assertEqual(
            first_request.full_url,
            "http://localhost:8080/api/v1/location-memories",
        )
        self.assertEqual(first_request.get_method(), "POST")
        self.assertEqual(
            first_payload,
            {
                "store_type": "restaurant",
                "store_name": "Juniper & Crumb Cafe",
                "latitude": 40.7580,
                "longitude": -73.9855,
                "message": "Usually order a latte.",
            },
        )
        self.assertEqual(
            json.loads(urlopen.call_args_list[1].args[0].data)["message"],
            "The croissants are great.",
        )

    def test_generates_coordinates_when_not_provided(self):
        with mock.patch(
            "seed_location_info.random.uniform", side_effect=[0.01, -0.01]
        ):
            urlopen = self.run_script(
                [
                    "--store-name",
                    "Market",
                    "--store-type",
                    "grocery",
                    "--message",
                    "Buy apples here.",
                ]
            )
        payload = json.loads(urlopen.call_args.args[0].data)
        self.assertAlmostEqual(payload["latitude"], 40.768)
        self.assertAlmostEqual(payload["longitude"], -73.9955)

    def test_rejects_only_one_coordinate(self):
        with self.assertRaises(SystemExit) as error:
            with contextlib.redirect_stderr(io.StringIO()):
                seed_location_info.main(
                    [
                        "--store-name",
                        "Market",
                        "--store-type",
                        "grocery",
                        "--message",
                        "Buy apples.",
                        "--latitude",
                        "37",
                    ]
                )
        self.assertEqual(error.exception.code, 2)

    def test_rejects_out_of_range_coordinates(self):
        with self.assertRaises(SystemExit) as error:
            with contextlib.redirect_stderr(io.StringIO()):
                seed_location_info.main(
                    [
                        "--store-name",
                        "Market",
                        "--store-type",
                        "grocery",
                        "--message",
                        "Buy apples.",
                        "--latitude",
                        "91",
                        "--longitude",
                        "0",
                    ]
                )
        self.assertEqual(error.exception.code, 2)

    def test_reports_http_failure(self):
        failure = HTTPError(
            "http://localhost:8080/api/v1/location-memories",
            400, "bad request", {}, io.BytesIO(b"invalid")
        )
        with mock.patch("seed_location_info.urllib.request.urlopen", side_effect=failure):
            with self.assertRaises(SystemExit) as error:
                with contextlib.redirect_stdout(io.StringIO()):
                    seed_location_info.main(
                        [
                            "--store-name",
                            "Market",
                            "--store-type",
                            "grocery",
                            "--message",
                            "Buy apples.",
                        ]
                    )
        self.assertIn("HTTP 400", str(error.exception))

    def test_reports_connection_failure(self):
        with mock.patch(
            "seed_location_info.urllib.request.urlopen",
            side_effect=URLError("connection refused"),
        ):
            with self.assertRaises(SystemExit) as error:
                with contextlib.redirect_stdout(io.StringIO()):
                    seed_location_info.main(
                        [
                            "--store-name",
                            "Market",
                            "--store-type",
                            "grocery",
                            "--message",
                            "Buy apples.",
                        ]
                    )
        self.assertIn("Could not reach", str(error.exception))


class SeedTaskTests(unittest.TestCase):
    def setUp(self):
        self.response = mock.MagicMock()
        self.response.__enter__.return_value = self.response
        self.response.status = 201
        self.response.read.return_value = b'{"id":1,"status":"pending"}'

    def run_script(self, arguments):
        with mock.patch(
            "seed_task.urllib.request.urlopen", return_value=self.response
        ) as urlopen:
            with contextlib.redirect_stdout(io.StringIO()):
                seed_task.main(arguments)
        return urlopen

    def test_sends_task_with_optional_fields(self):
        urlopen = self.run_script(
            [
                "--title",
                "Buy milk",
                "--description",
                "2% milk",
                "--due-at",
                "2026-10-17T09:30:00Z",
                "--endpoint",
                "http://localhost:9090/api/v1/tasks",
            ]
        )

        request = urlopen.call_args.args[0]
        self.assertEqual(request.full_url, "http://localhost:9090/api/v1/tasks")
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(
            json.loads(request.data),
            {
                "title": "Buy milk",
                "description": "2% milk",
                "due_at": "2026-10-17T09:30:00Z",
            },
        )

    def test_omits_due_date_when_not_provided(self):
        urlopen = self.run_script(["--title", "Clean kitchen"])
        payload = json.loads(urlopen.call_args.args[0].data)
        self.assertEqual(payload, {"title": "Clean kitchen", "description": ""})

    def test_requires_title(self):
        with self.assertRaises(SystemExit) as error:
            with contextlib.redirect_stderr(io.StringIO()):
                seed_task.main([])
        self.assertEqual(error.exception.code, 2)

    def test_reports_http_failure(self):
        failure = HTTPError(
            "http://localhost:8080/api/v1/tasks",
            500, "server error", {}, io.BytesIO(b"failed")
        )
        with mock.patch("seed_task.urllib.request.urlopen", side_effect=failure):
            with self.assertRaises(SystemExit) as error:
                with contextlib.redirect_stdout(io.StringIO()):
                    seed_task.main(["--title", "Buy milk"])
        self.assertIn("HTTP 500", str(error.exception))

    def test_reports_connection_failure(self):
        with mock.patch(
            "seed_task.urllib.request.urlopen", side_effect=URLError("connection refused")
        ):
            with self.assertRaises(SystemExit) as error:
                with contextlib.redirect_stdout(io.StringIO()):
                    seed_task.main(["--title", "Buy milk"])
        self.assertIn("Could not reach", str(error.exception))


if __name__ == "__main__":
    unittest.main()
