// Program Trace Analysis with Wireshark --- Unmodified Example Program

#include <cstdint>
#include <cstdlib>

struct plugin_response {
  uint32_t tag;
  uint32_t z;
};

struct plugin_request {
  uint32_t tag;
  uint32_t x;
  uint32_t y;
};

void do_plugin_request(plugin_request *req, plugin_response *resp) {
  // Suspend your disbelief...  Pretend this is a complicated plugin system.
  resp->tag = req->tag;
  resp->z = req->x + req->y;
}

int main(int argc, char **argv) {
  for (int i = 0; i < 1000; ++i) {
    plugin_request req = {uint32_t(i), uint32_t(rand()), uint32_t(rand())};
    plugin_response resp;
    do_plugin_request(&req, &resp);
  }

  return 0;
}
