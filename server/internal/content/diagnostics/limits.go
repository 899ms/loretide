package diagnostics
import("fmt";"strconv";"time")
// ConfigureLimits accepts bounded values; invalid configuration is never ignored.
func(s *Store)ConfigureLimits(maxLogs,days string)error{
 if maxLogs!=""{n,err:=strconv.Atoi(maxLogs);if err!=nil||n<100||n>100000{return fmt.Errorf("LORETIDE_DIAG_MAX_LOGS must be 100..100000")};s.MaxLogs=n}
 if days!=""{n,err:=strconv.Atoi(days);if err!=nil||n<1||n>90{return fmt.Errorf("LORETIDE_DIAG_RETENTION_DAYS must be 1..90")};s.Retention=time.Duration(n)*24*time.Hour};return nil
}
