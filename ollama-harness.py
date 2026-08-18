import base64
import io
import json
import logging
import os
import urllib.error
import urllib.request
from typing import Any, Dict, Optional
from PIL import Image

# Configure structured logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s - %(message)s",
    handlers=[
        logging.StreamHandler()
    ]
)
logger = logging.getLogger("WhisperVMBrainHarness")


class WhisperVMBrainHarness:
    def __init__(
        self,
        model_name: str = "llama3",
        host: str = "http://localhost:11434",
        current_frame: str = "whisperframe_output.png"
    ) -> None:
        self.model_name = model_name
        self.host = host.rstrip("/")
        self.current_frame = current_frame
        self.generate_endpoint = f"{self.host}/api/generate"
        logger.info("Initialized WhisperVMBrainHarness (Model: %s, Endpoint: %s)", self.model_name, self.generate_endpoint)

    def _encode_frame(self, frame_path: str) -> Optional[str]:
        """Loads a frame PNG image and converts it into a base64-encoded string."""
        if not os.path.exists(frame_path):
            logger.warning("Frame file '%s' not found. Skipping visual frame attachment.", frame_path)
            return None

        try:
            with Image.open(frame_path) as img:
                buffered = io.BytesIO()
                # Ensure compatibility by saving as PNG
                img.save(buffered, format="PNG")
                encoded_string = base64.b64encode(buffered.getvalue()).decode("utf-8")
                logger.info("Successfully encoded frame '%s' to base64.", frame_path)
                return encoded_string
        except Exception as e:
            logger.error("Failed to process image frame '%s': %s", frame_path, str(e))
            return None

    def _graceful_fallback(self, error_message: str, prompt: str) -> Dict[str, Any]:
        """Provides a safe, structured fallback response if the model or connection fails."""
        logger.warning("Triggering fallback routine. Reason: %s", error_message)
        return {
            "status": "fallback",
            "model": self.model_name,
            "prompt": prompt,
            "response": f"[FALLBACK STATE] Execution bypassed due to failure: {error_message}",
            "done": True
        }

    def step(self, prompt: str, frame_path: Optional[str] = None) -> Dict[str, Any]:
        """
        Executes a single step cycle: builds the payload (including framed PNG data if present),
        sends it to Ollama, and handles responses/fallbacks.
        """
        target_frame = frame_path or self.current_frame
        logger.info("Executing step with prompt: '%s'", prompt)

        # Build base payload
        payload: Dict[str, Any] = {
            "model": self.model_name,
            "prompt": prompt,
            "stream": False
        }

        # Encode and attach image frame if available
        encoded_image = self._encode_frame(target_frame)
        if encoded_image:
            payload["images"] = [encoded_image]

        data = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            self.generate_endpoint,
            data=data,
            headers={"Content-Type": "application/json"}
        )

        try:
            logger.info("Sending request to Ollama endpoint: %s", self.generate_endpoint)
            with urllib.request.urlopen(req, timeout=60) as response:
                if response.status == 200:
                    response_body = response.read().decode("utf-8")
                    result = json.loads(response_body)
                    logger.info("Step completed successfully.")
                    return {
                        "status": "success",
                        "response": result.get("response", ""),
                        "raw": result
                    }
                else:
                    logger.error("Received unexpected HTTP status: %d", response.status)
                    return self._graceful_fallback(f"Unexpected HTTP status {response.status}", prompt)

        except urllib.error.HTTPError as http_err:
            logger.error("HTTP error occurred while contacting Ollama: %d %s", http_err.code, http_err.reason)
            return self._graceful_fallback(f"HTTPError {http_err.code}: {http_err.reason}", prompt)

        except urllib.error.URLError as url_err:
            logger.error("Network or connection error: %s. Is Ollama running on %s?", url_err.reason, self.host)
            return self._graceful_fallback(f"URLError: {url_err.reason}", prompt)

        except TimeoutError:
            logger.error("Request to Ollama timed out.")
            return self._graceful_fallback("Request timeout after 60 seconds", prompt)

        except json.JSONDecodeError as json_err:
            logger.error("Failed to decode response JSON from Ollama: %s", str(json_err))
            return self._graceful_fallback(f"JSONDecodeError: {str(json_err)}", prompt)

        except Exception as ex:
            logger.error("Unexpected error during execution step: %s", str(ex))
            return self._graceful_fallback(f"Unexpected Exception: {str(ex)}", prompt)


if __name__ == "__main__":
    # Initialize harness instance
    brain = WhisperVMBrainHarness(
        model_name="llama3",
        host="http://localhost:11434",
        current_frame="whisperframe_output.png"
    )

    # Execute step sequence
    execution_result = brain.step("Initialize system memory and execute core sequence.")
    logger.info("Output Response: %s", execution_result.get("response"))
