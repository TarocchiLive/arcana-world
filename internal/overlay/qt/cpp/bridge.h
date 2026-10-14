#ifndef ARCANA_AWQT_BRIDGE_H
#define ARCANA_AWQT_BRIDGE_H

#if defined(_WIN32)
# if defined(AWQT_BUILD)
#  define AWQT_API __declspec(dllexport)
# else
#  define AWQT_API __declspec(dllimport)
# endif
#else
# define AWQT_API __attribute__((visibility("default")))
#endif

#ifdef __cplusplus
extern "C" {
#endif
/* create、run、displays 必须在主操作系统线程调用；成功返回非零。 */
AWQT_API int awqt_create(const char *json);
AWQT_API int awqt_run(void);
/* update、stop 可跨线程调用；输入在返回前完成复制。 */
AWQT_API void awqt_update(const char *json);
AWQT_API void awqt_stop(void);
/* 返回的 UTF-8 JSON 由 awqt_free 释放。错误字符串保留到下次读取。 */
AWQT_API char *awqt_displays(const char *json);
AWQT_API void awqt_free(char *value);
AWQT_API const char *awqt_error(void);
#ifdef __cplusplus
}
#endif
#endif
